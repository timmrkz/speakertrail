package speech

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/fetch"
)

// Unknown marks a fraction or a remaining time that cannot be given yet.
const Unknown = fetch.Unknown

// Progress is how far an install has come, see fetch.Progress.
type Progress = fetch.Progress

// ErrCancelled is an install stopped by hand. What arrived stays, and the
// next Download carries on from it.
var ErrCancelled = fetch.ErrCancelled

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

// getter is the one download every model arrives through.
func (in Installer) getter() fetch.Getter {
	return fetch.Getter{Client: in.Client, Report: in.Report, Now: in.Now}
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
	sum, err := in.getter().Check(ctx, part, m.Download, "checking")
	if err != nil {
		return err
	}
	if m.SHA256 == "" || !strings.EqualFold(sum, m.SHA256) {
		if sum, err = in.getter().Get(ctx, m.URL, m.Download, part, m.Title); err != nil {
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
	return fetch.UnpackTarBz2(ctx, archive, into, func(n int64) {
		done += n
		if now := in.now(); now.Sub(last) >= time.Second {
			last = now
			in.report(say(now))
		}
	})
}

// Remove deletes an installed model, its folder, and whatever a download
// or an unpacking of it left behind. A name is only ever taken inside dir,
// judged by where it really leads, so a name that somehow pointed
// elsewhere deletes nothing. What is already gone is fine.
func Remove(m Model, dir string) error {
	for _, name := range []string{m.Name, m.Name + ".part", m.Name + ".unpacking"} {
		path, err := fetch.SafeChild(dir, name)
		if err != nil {
			return err
		}
		if path == fetch.Resolve(dir) {
			return errors.New("refusing to remove the models folder itself")
		}
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
