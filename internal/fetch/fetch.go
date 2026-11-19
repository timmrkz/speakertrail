// Package fetch brings a large file over the network the way the app
// fetches its models: carried on where a stopped download left off,
// checked against the SHA-256 it should have, never more than it should
// weigh, and saying how far it has come about once a second. The speech
// model and the language model both arrive through it, as both arrive
// through one download in FrameFairy's engine.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Unknown marks a fraction or a remaining time that cannot be given yet.
const Unknown = -1.0

// Progress is how far a download has come, the way FrameFairy's job says
// it: what it does now, the share done from 0 to 1, and the seconds left.
type Progress struct {
	Text      string  `json:"text"`
	Fraction  float64 `json:"fraction"`
	Remaining float64 `json:"remaining"`
}

// ErrCancelled is a download stopped by hand. What arrived stays, and the
// next try carries on from it.
var ErrCancelled = errors.New("cancelled")

// Getter fetches. The zero value is what the app uses.
type Getter struct {
	// Client fetches the file, http.DefaultClient when nil.
	Client *http.Client
	// Report hears how far it has come, about once a second, and nil
	// once there is nothing to say.
	Report func(*Progress)
	// Now is the clock the reports are timed by, time.Now when nil.
	Now func() time.Time
}

func (in Getter) report(p *Progress) {
	if in.Report != nil {
		in.Report(p)
	}
}

func (in Getter) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

// Get writes what is at url to path and returns its checksum, taken
// as the bytes go past.
//
// It carries on where a stopped download left off. What is already in
// path is what was already fetched, so the rest is asked for with a range
// and the bytes on disk go through the checksum first. A server that will
// not do ranges answers with the whole file, which is then written again
// from the start.
//
// expect is how big it should be, used to report how far the download has
// come when the server does not say, to know a download that stopped
// early from one that finished, and to stop one that goes on past it.
// what is the name to say it by.
func (in Getter) Get(ctx context.Context, url string, expect int64, path, what string) (string, error) {
	have := int64(0)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		have = info.Size()
	}
	// A part file as big as the whole thing is a download that finished
	// and was never checked. Ask for the lot rather than for nothing.
	if expect > 0 && have >= expect {
		have = 0
	}

	// A connection can go quiet without closing. Every byte that arrives
	// puts the watch back, and one that stays quiet for downloadStall ends
	// the download, with what came kept to carry on from.
	outer := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var quiet atomic.Bool
	watch := time.AfterFunc(downloadStall, func() {
		quiet.Store(true)
		cancel()
	})
	defer watch.Stop()
	stalled := func(err error) error {
		if quiet.Load() && outer.Err() == nil {
			return fmt.Errorf("%s stopped arriving for %s. What came is kept, so trying again carries on from there.",
				what, downloadStall)
		}
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	client := in.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		if outer.Err() != nil {
			return "", ErrCancelled
		}
		if e := stalled(nil); e != nil {
			return "", e
		}
		return "", fmt.Errorf("could not reach %s: %w", what, err)
	}
	defer res.Body.Close()
	body := watchedReader{res.Body, func() { watch.Reset(downloadStall) }}
	switch res.StatusCode {
	case http.StatusPartialContent:
		// Carrying on.
	case http.StatusOK:
		// The range was not honoured, so this is the whole file again.
		have = 0
	default:
		return "", fmt.Errorf("%s answered %s", what, res.Status)
	}
	// Never more than the model is known to weigh, and a little room, so
	// a server that never stops sending cannot fill the disk.
	limit := int64(-1)
	if expect > 0 {
		limit = expect + downloadRoom(expect)
		if res.ContentLength > 0 && have+res.ContentLength > limit {
			return "", fmt.Errorf("%s is %s, more than the %s it should be, so it was not fetched.",
				what, InMB(have+res.ContentLength), InMB(expect))
		}
		body.Reader = io.LimitReader(body.Reader, limit-have+1)
	}

	sum := sha256.New()
	var file *os.File
	if have > 0 {
		if file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
			return "", err
		}
		// The sum is of the whole file, so what is already on disk goes
		// through it before anything new does.
		if err := hashInto(sum, path, have); err != nil {
			file.Close()
			return "", err
		}
		in.report(&Progress{Text: fmt.Sprintf("carrying on from %s", InMB(have)), Fraction: float64(have) / float64(max(expect, have+1)), Remaining: Unknown})
	} else if file, err = os.Create(path); err != nil {
		return "", err
	}
	defer file.Close()

	total := res.ContentLength + have
	if res.ContentLength <= 0 {
		total = expect
	}
	done, err := in.copyWithProgress(ctx, io.MultiWriter(file, sum), body, have, total)
	if err != nil {
		if outer.Err() != nil {
			return "", ErrCancelled
		}
		return "", stalled(err)
	}
	if limit >= 0 && done > limit {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("%s went on past the %s it should be, so it was stopped and thrown away.",
			what, InMB(expect))
	}
	if total > 0 && done < total {
		return "", fmt.Errorf("%s stopped after %s of %s.", what, InMB(done), InMB(total))
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// Arrived says whether the file at path is the whole of a download of
// size bytes whose SHA-256 is sha, judged by its size and its checksum,
// and gives the checksum. A download that finished and was stopped before
// it was put in place is then not fetched again.
func Arrived(path string, size int64, sha string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || sha == "" || info.Size() != size {
		return "", false
	}
	sum := sha256.New()
	if err := hashInto(sum, path, info.Size()); err != nil {
		return "", false
	}
	got := hex.EncodeToString(sum.Sum(nil))
	return got, strings.EqualFold(got, sha)
}

// downloadRoom is how far past its known size a download may go: a
// hundredth of it, and never less than a megabyte.
func downloadRoom(expect int64) int64 {
	return max(expect/100, 1<<20)
}

// downloadStall is how long a download may go without a byte before it is
// taken for dead.
const downloadStall = time.Minute

// watchedReader calls arrived each time bytes come.
type watchedReader struct {
	io.Reader
	arrived func()
}

func (r watchedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.arrived()
	}
	return n, err
}

// hashInto feeds the first n bytes of a file to a checksum.
func hashInto(sum io.Writer, path string, n int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(sum, io.LimitReader(file, n))
	return err
}

// copyWithProgress is io.Copy that says how far it has got, about once a
// second. done starts at what was already there, so a download carrying
// on shows how far the file has come rather than how far this attempt has.
func (in Getter) copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, already, total int64) (int64, error) {
	buf := make([]byte, 256*1024)
	done := already
	began := in.now()
	last := began
	for {
		if err := ctx.Err(); err != nil {
			return done, ErrCancelled
		}
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			done += int64(written)
			if werr != nil {
				return done, werr
			}
		}
		if now := in.now(); now.Sub(last) >= time.Second {
			last = now
			if total > 0 {
				left := Unknown
				speed := ""
				if rate := float64(done-already) / now.Sub(began).Seconds(); rate > 0 {
					left = float64(total-done) / rate
					speed = fmt.Sprintf(" at %s a second", InMB(int64(rate)))
				}
				in.report(&Progress{Text: fmt.Sprintf("fetching %s of %s%s", InMB(done), InMB(total), speed),
					Fraction: float64(done) / float64(total), Remaining: left})
			} else {
				in.report(&Progress{Text: fmt.Sprintf("fetching, %s", InMB(done)), Fraction: Unknown, Remaining: Unknown})
			}
		}
		if err == io.EOF {
			return done, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return done, ErrCancelled
			}
			return done, err
		}
	}
}

// InMB says how much has been fetched the way the card says how big a
// download is, in thousands rather than in powers of two, so the numbers
// in one row agree.
func InMB(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%d MB", n/1e6)
	}
	return fmt.Sprintf("%d KB", n/1e3)
}

// Resolve is path with every link in it followed, as far as the path
// exists, and the rest joined on.
func Resolve(path string) string {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(Resolve(parent), filepath.Base(path))
}

// SafeChild joins a name onto a folder and checks that the result stays
// inside it, judged by where it really leads.
func SafeChild(base, name string) (string, error) {
	root := Resolve(base)
	joined := Resolve(filepath.Join(base, name))
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing to write outside %s: %q", base, name)
	}
	return joined, nil
}
