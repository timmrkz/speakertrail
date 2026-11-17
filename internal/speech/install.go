package speech

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Unknown marks a fraction or a remaining time that cannot be given yet.
const Unknown = -1.0

// Progress is how far an install has come, the way FrameFairy's job says
// it: what it does now, the share done from 0 to 1, and the seconds left.
type Progress struct {
	Text      string  `json:"text"`
	Fraction  float64 `json:"fraction"`
	Remaining float64 `json:"remaining"`
}

// ErrCancelled is an install stopped by hand. What arrived stays, and the
// next Download carries on from it.
var ErrCancelled = errors.New("cancelled")

// Installer fetches and unpacks a model. The zero value is what the app
// uses.
type Installer struct {
	// Client fetches the archive, http.DefaultClient when nil.
	Client *http.Client
	// Report hears how far it has come, about once a second, and nil
	// once there is nothing to say.
	Report func(*Progress)
	// Now is the clock the reports are timed by, time.Now when nil.
	Now func() time.Time
}

func (in Installer) report(p *Progress) {
	if in.Report != nil {
		in.Report(p)
	}
}

func (in Installer) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

// Install fetches a model and unpacks it into dir, reporting as it goes.
//
// It is written to be interrupted. The download lands beside the folder
// under a part name, the unpacking goes to a folder of its own, and only a
// complete model is moved into place, so a Mac that loses power or a press
// of Cancel leaves no model rather than half of one.
func (in Installer) Install(ctx context.Context, m Model, dir string) error {
	if m.Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(dir, m.Name+".part")
	staging := filepath.Join(dir, m.Name+".unpacking")
	final := filepath.Join(dir, m.Name)
	// Half an unpacking is never worth keeping. Half a download is: it is
	// what a second try carries on from, rather than beginning again at
	// nothing.
	defer os.RemoveAll(staging)
	defer in.report(nil)

	// Bytes that turned out to be the wrong ones go, so a second try does
	// not carry on from them for ever.
	wrong := func(err error) error {
		os.Remove(part)
		return err
	}

	// An archive that arrived whole and was stopped while it unpacked is
	// not fetched again: what is on disk is checked and unpacked as it is.
	sum, ok := arrived(part, m)
	if !ok {
		var err error
		if sum, err = in.download(ctx, m.URL, m.Download, part, m.Title); err != nil {
			return err
		}
	}
	if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
		return wrong(fmt.Errorf("%s did not arrive as expected. It should be %s and came to %s.",
			m.Title, m.SHA256, sum))
	}

	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := in.unpack(ctx, part, staging, m.Unpacked); err != nil {
		// An archive that will not unpack is an archive that is wrong,
		// whatever its checksum said, and there is nothing in it to carry
		// on from.
		if ctx.Err() == nil {
			return wrong(err)
		}
		return err
	}

	// The archive holds one folder named after the model. Take what is
	// inside it rather than nesting it a second time.
	inner := filepath.Join(staging, m.Name)
	if _, err := os.Stat(inner); err != nil {
		inner = staging
	}
	if _, err := os.Stat(filepath.Join(inner, "tokens.txt")); err != nil {
		return wrong(fmt.Errorf("%s unpacked without a tokens.txt, so it is not the model it claims to be.", m.Title))
	}
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	if err := os.Rename(inner, final); err != nil {
		return err
	}
	// The archive has done its work, so it goes.
	os.Remove(part)
	return nil
}

// download writes what is at url to path and returns its checksum, taken
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
func (in Installer) download(ctx context.Context, url string, expect int64, path, what string) (string, error) {
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

// arrived says whether the archive at part is the whole of m, judged by
// its size and its checksum, and gives the checksum.
func arrived(part string, m Model) (string, bool) {
	info, err := os.Stat(part)
	if err != nil || !info.Mode().IsRegular() || m.SHA256 == "" || info.Size() != m.Download {
		return "", false
	}
	sum := sha256.New()
	if err := hashInto(sum, part, info.Size()); err != nil {
		return "", false
	}
	got := hex.EncodeToString(sum.Sum(nil))
	return got, strings.EqualFold(got, m.SHA256)
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
func (in Installer) copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, already, total int64) (int64, error) {
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

// unpack writes an archive into a folder and says how far it has come,
// about once a second: what is written against total, what the model is
// known to take once unpacked, and the time left at the pace it has kept
// so far. Unpacking takes over a minute on a fast Mac, so it is shown
// the way the download is rather than as work with no end in sight.
// FrameFairy says only "unpacking" here.
func (in Installer) unpack(ctx context.Context, archive, into string, total int64) error {
	began := in.now()
	last := began
	done := int64(0)
	say := func(now time.Time) *Progress {
		p := &Progress{Text: "unpacking", Fraction: Unknown, Remaining: Unknown}
		if total <= 0 {
			return p
		}
		p.Fraction = min(float64(done)/float64(total), 1)
		if rate := float64(done) / now.Sub(began).Seconds(); rate > 0 && done < total {
			p.Remaining = float64(total-done) / rate
		}
		return p
	}
	in.report(say(began))
	return unpackTarBz2(ctx, archive, into, func(n int64) {
		done += n
		if now := in.now(); now.Sub(last) >= time.Second {
			last = now
			in.report(say(now))
		}
	})
}

// unpackTarBz2 writes an archive into a folder, refusing anything that
// would land outside it. wrote, when given, hears every piece written.
//
// What arrives over a network is untrusted. A tar can name ../../../ and
// a link can point anywhere, and both are how an archive writes over
// something it was never given. Every name goes through safeChild, and no
// link of either kind is written at all.
//
// A model is mostly one file of several hundred megabytes, so a stop is
// heard between the pieces of a file, not only between files.
func unpackTarBz2(ctx context.Context, archive, into string, wrote func(int64)) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	reader := tar.NewReader(bzip2.NewReader(file))
	for {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		head, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the archive could not be read: %w", err)
		}
		switch head.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		default:
			// Links, devices, anything else. Skipped rather than refused,
			// so one odd entry does not throw away a good model.
			continue
		}
		target, err := safeChild(into, head.Name)
		if err != nil {
			return err
		}
		if head.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		// Bounded by what the header says, so a stream that claims one
		// size and keeps going cannot fill the disk.
		if _, err := io.Copy(out, io.LimitReader(heeding{ctx, reader, wrote}, head.Size)); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

// heeding reads on until ctx is done, and tells wrote how much each read
// brought.
type heeding struct {
	ctx   context.Context
	r     io.Reader
	wrote func(int64)
}

func (h heeding) Read(p []byte) (int, error) {
	if h.ctx.Err() != nil {
		return 0, ErrCancelled
	}
	n, err := h.r.Read(p)
	if n > 0 && h.wrote != nil {
		h.wrote(int64(n))
	}
	return n, err
}

// resolve is path with every link in it followed, as far as the path
// exists, and the rest joined on.
func resolve(path string) string {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolve(parent), filepath.Base(path))
}

// safeChild joins a name onto a folder and checks that the result stays
// inside it, judged by where it really leads.
func safeChild(base, name string) (string, error) {
	root := resolve(base)
	joined := resolve(filepath.Join(base, name))
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing to write outside %s: %q", base, name)
	}
	return joined, nil
}

// Remove deletes an installed model, its folder, and whatever a download
// or an unpacking of it left behind. A name is only ever taken inside dir,
// judged by where it really leads, so a name that somehow pointed
// elsewhere deletes nothing. What is already gone is fine.
func Remove(m Model, dir string) error {
	for _, name := range []string{m.Name, m.Name + ".part", m.Name + ".unpacking"} {
		path, err := safeChild(dir, name)
		if err != nil {
			return err
		}
		if path == resolve(dir) {
			return errors.New("refusing to remove the models folder itself")
		}
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
