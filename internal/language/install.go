package language

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/fetch"
)

// Installer fetches a model. The zero value is what the app uses.
type Installer struct {
	// Client fetches the file, http.DefaultClient when nil.
	Client *http.Client
	// Report hears how far it has come, about once a second, and nil
	// once there is nothing to say.
	Report func(*fetch.Progress)
	// Now is the clock the reports are timed by, time.Now when nil.
	Now func() time.Time
	// Elsewhere are folders another app keeps the same models in,
	// FrameFairy's ~/.framefairy/models. A model found there whole, by its
	// checksum, is taken from there rather than downloaded, see take.
	Elsewhere []string
}

// FrameFairyModels is where FrameFairy keeps its models, the same files
// with the same checksums. Empty when there is no home folder.
func FrameFairyModels() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".framefairy", "models")
}

// Install fetches a model into dir, reporting as it goes.
//
// It is written to be interrupted, the way FrameFairy's is: the download
// lands under a part name and is only moved into place once it is whole
// and checked, so a Mac that loses power or a press of Cancel leaves no
// model rather than half of one. Half a model is worse than none, because
// it looks installed.
func (in Installer) Install(ctx context.Context, m Model, dir string) error {
	if m.Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(dir, m.Name+".part")
	final := filepath.Join(dir, m.Name)
	defer func() {
		if in.Report != nil {
			in.Report(nil)
		}
	}()
	// Bytes that turned out to be the wrong ones go, so a second try does
	// not carry on from them for ever. Bytes that simply stopped arriving
	// stay, because that is what makes the next try carry on.
	wrong := func(err error) error {
		os.Remove(part)
		return err
	}
	took, err := in.take(ctx, m, part)
	if err != nil {
		return err
	}
	if took {
		if err := os.Rename(part, final); err != nil {
			return err
		}
		noteChecked(dir, m, final)
		return nil
	}
	// A download that arrived whole and was stopped before it was moved
	// into place is checked rather than fetched again.
	g := fetch.Getter{Client: in.Client, Report: in.Report, Now: in.Now}
	sum, err := g.Check(ctx, part, m.Download, "checking")
	if err != nil {
		return err
	}
	if m.SHA256 == "" || !strings.EqualFold(sum, m.SHA256) {
		if sum, err = g.Get(ctx, m.URL, m.Download, part, m.Title); err != nil {
			return err
		}
	}
	if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
		return wrong(fmt.Errorf("%s did not arrive as expected. It should be %s and came to %s.", m.Title, m.SHA256, sum))
	}
	// A page saying no, saved under a model's name, is what this catches.
	if !isGGUF(part) {
		return wrong(fmt.Errorf("what came back from %s is not a model file.", m.URL))
	}
	if err := os.Rename(part, final); err != nil {
		return err
	}
	noteChecked(dir, m, final)
	return nil
}

// Remove deletes an installed model and whatever a download of it left
// behind. A name is only ever taken inside dir, judged by where it really
// leads. What is already gone is fine.
func Remove(m Model, dir string) error {
	for _, name := range []string{m.Name, m.Name + ".part"} {
		path, err := fetch.SafeChild(dir, name)
		if err != nil {
			return err
		}
		if path == fetch.Resolve(dir) {
			return errors.New("refusing to remove the models folder itself")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	forgetChecked(dir, m)
	return nil
}

// A model the app downloaded is checked again before it is loaded. Its
// SHA-256 is checked when it arrives, but the file then lies in the
// models folder for months, and llama-server reads it with no questions.
// Reading fifteen gigabytes takes several seconds, so what a check found
// is kept beside the models, with the size and the time of the file then,
// and the file is read again only when either has moved. As in
// FrameFairy's modelcheck.go.

// checkedFile is the file in the models folder that keeps what the checks
// found.
const checkedFile = ".checked.json"

type checked struct {
	Size   int64     `json:"size"`
	Mod    time.Time `json:"mod"`
	SHA256 string    `json:"sha256"`
}

// checks guards the record, which a download and a load may both write.
var checks sync.Mutex

// Check says whether the model m in dir may be loaded: it has to be the
// file its SHA-256 says.
func Check(m Model, dir string) error {
	path := filepath.Join(dir, m.Name)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	checks.Lock()
	defer checks.Unlock()
	record := readChecked(dir)
	was, seen := record[m.Name]
	if seen && was.Size == info.Size() && was.Mod.Equal(info.ModTime()) && strings.EqualFold(was.SHA256, m.SHA256) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	sum := sha256.New()
	_, err = io.Copy(sum, file)
	file.Close()
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, m.SHA256) {
		return fmt.Errorf("%s is not the file it was when it was downloaded, so it is not loaded. Remove it in Settings and download it again.", m.Title)
	}
	record[m.Name] = checked{Size: info.Size(), Mod: info.ModTime(), SHA256: m.SHA256}
	writeChecked(dir, record)
	return nil
}

// noteChecked keeps what the download of a model has just checked, so
// the first draft after it does not read the whole file a second time.
func noteChecked(dir string, m Model, path string) {
	info, err := os.Stat(path)
	if err != nil || m.SHA256 == "" {
		return
	}
	checks.Lock()
	defer checks.Unlock()
	record := readChecked(dir)
	record[m.Name] = checked{Size: info.Size(), Mod: info.ModTime(), SHA256: m.SHA256}
	writeChecked(dir, record)
}

func forgetChecked(dir string, m Model) {
	checks.Lock()
	defer checks.Unlock()
	record := readChecked(dir)
	if _, ok := record[m.Name]; ok {
		delete(record, m.Name)
		writeChecked(dir, record)
	}
}

func readChecked(dir string) map[string]checked {
	record := map[string]checked{}
	if body, err := os.ReadFile(filepath.Join(dir, checkedFile)); err == nil {
		_ = json.Unmarshal(body, &record)
	}
	return record
}

// writeChecked writes the record whole to a new file that then replaces
// the old one. One that cannot be written only means the next check reads
// the model again.
func writeChecked(dir string, record map[string]checked) {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return
	}
	tmp := filepath.Join(dir, checkedFile+".new")
	if os.WriteFile(tmp, body, 0o644) == nil {
		_ = os.Rename(tmp, filepath.Join(dir, checkedFile))
	}
}

// take puts the model at part from another app's folder, when it is there
// whole: the same size, the same checksum and a model file. It is a hard
// link, so the two apps share the bytes on disk, 14.4 GB for Gemma 4 26B,
// and either can remove its own name without taking the other's. A folder
// on another volume cannot be linked from, and the model is downloaded.
// Nothing is ever written in the other app's folder.
//
// Reading all of it to check it takes seconds, so it says how far it is,
// and Cancel stops it.
func (in Installer) take(ctx context.Context, m Model, part string) (bool, error) {
	for _, dir := range in.Elsewhere {
		if dir == "" || m.SHA256 == "" {
			continue
		}
		there := filepath.Join(dir, m.Name)
		info, err := os.Stat(there)
		if err != nil || !info.Mode().IsRegular() || info.Size() != m.Download || !isGGUF(there) {
			continue
		}
		if in.Report != nil {
			in.Report(&fetch.Progress{Text: "copying", Fraction: fetch.Unknown, Remaining: fetch.Unknown})
		}
		g := fetch.Getter{Report: in.Report, Now: in.Now}
		sum, err := g.Check(ctx, there, m.Download, "copying")
		if err != nil {
			return false, err
		}
		if !strings.EqualFold(sum, m.SHA256) {
			continue
		}
		os.Remove(part)
		if os.Link(there, part) == nil {
			return true, nil
		}
	}
	return false, nil
}
