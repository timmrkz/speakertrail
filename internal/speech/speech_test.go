package speech

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Ported from FrameFairy's engine/speech_test.go.

// A tar.bz2 the way the real one comes: one folder named after the model,
// with tokens.txt inside it. bzip2 has no writer in the standard library,
// so the system's is used and the test skips without it.
func modelArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	for name, body := range entries {
		head := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if body == "" && strings.HasSuffix(name, "/") {
			head = &tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := w.WriteHeader(head); err != nil {
			t.Fatal(err)
		}
		if head.Typeflag == tar.TypeReg {
			if _, err := w.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bzip(t, plain.Bytes())
}

func bzip(t *testing.T, plain []byte) []byte {
	t.Helper()
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 is not installed")
	}
	cmd := exec.Command("bzip2", "-c")
	cmd.Stdin = bytes.NewReader(plain)
	packed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return packed
}

// Serves an archive, so the whole install runs without reaching the real
// internet. Tests need no network.
func serving(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

// On the real disk: the speech model is unpacked into the folder the speech library loads.
func TestInstallingASpeechModel(t *testing.T) {
	const name = "test-model"
	good := modelArchive(t, map[string]string{
		name + "/tokens.txt": "a 0\nb 1\n",
		name + "/model.onnx": "not really a model",
	})

	t.Run("it arrives, unpacks and is usable", func(t *testing.T) {
		dir := t.TempDir()
		s := serving(t, good)
		m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good))}
		if m.Installed(dir) {
			t.Fatal("said it was there before anything happened")
		}
		if err := (Installer{}).Install(context.Background(), m, dir); err != nil {
			t.Fatal(err)
		}
		if !m.Installed(dir) {
			t.Fatal("installed and then said it was not there")
		}
		body, err := os.ReadFile(filepath.Join(dir, name, "tokens.txt"))
		if err != nil || len(body) == 0 {
			t.Fatalf("tokens.txt: %v", err)
		}
		// Nothing half finished is left lying about.
		for _, leftover := range []string{name + ".part", name + ".unpacking"} {
			if _, err := os.Stat(filepath.Join(dir, leftover)); err == nil {
				t.Errorf("%s was left behind", leftover)
			}
		}
		if got, ok := InUse(Models(), dir); ok {
			t.Errorf("the test model is in no list, and %s was taken for the one in use", got.Name)
		}
	})

	t.Run("installing again does nothing and does not fetch", func(t *testing.T) {
		dir := t.TempDir()
		asked := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked++
			_, _ = w.Write(good)
		}))
		defer s.Close()
		m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good))}
		for range 2 {
			if err := (Installer{}).Install(context.Background(), m, dir); err != nil {
				t.Fatal(err)
			}
		}
		if asked != 1 {
			t.Errorf("fetched %d times, want 1", asked)
		}
	})

	// What arrives over a network is untrusted. The checksum is the whole
	// reason the model is not unpacked straight from the socket.
	t.Run("a model that is not what it claims is refused", func(t *testing.T) {
		dir := t.TempDir()
		s := serving(t, good)
		m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good)),
			SHA256: strings.Repeat("0", 64)}
		if err := (Installer{}).Install(context.Background(), m, dir); err == nil {
			t.Fatal("took a model with the wrong checksum")
		}
		if m.Installed(dir) {
			t.Error("and unpacked it anyway")
		}
		if _, err := os.Stat(filepath.Join(dir, name+".part")); err == nil {
			t.Error("and kept the wrong bytes to carry on from")
		}
	})

	t.Run("an archive without the model in it is refused", func(t *testing.T) {
		dir := t.TempDir()
		wrong := modelArchive(t, map[string]string{"readme.txt": "nothing to see"})
		s := serving(t, wrong)
		m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(wrong))}
		if err := (Installer{}).Install(context.Background(), m, dir); err == nil {
			t.Fatal("took an archive with no tokens.txt")
		}
		if m.Installed(dir) {
			t.Error("and said it was installed")
		}
	})

	t.Run("a server that says no leaves nothing behind", func(t *testing.T) {
		dir := t.TempDir()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusNotFound)
		}))
		defer s.Close()
		m := Model{Name: name, Title: "Test", URL: s.URL}
		if err := (Installer{}).Install(context.Background(), m, dir); err == nil {
			t.Fatal("took a 404 for a model")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Errorf("left %d things behind", len(entries))
		}
	})

	// Cancel has to leave no model rather than half of one, because half a
	// model looks installed and fails on the first second of audio.
	t.Run("cancelling leaves no model at all", func(t *testing.T) {
		dir := t.TempDir()
		ctx, stop := context.WithCancel(context.Background())
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-length", strconv.Itoa(len(good)*40))
			_, _ = w.Write(good)
			stop()
			// Keep sending, so the copy is inside the loop when the stop
			// lands rather than already finished.
			for range 39 {
				if _, err := w.Write(good); err != nil {
					return
				}
			}
		}))
		defer s.Close()
		m := Model{Name: name, Title: "Test", URL: s.URL}
		err := (Installer{}).Install(ctx, m, dir)
		if err != ErrCancelled {
			t.Fatalf("a cancelled install said %v, want ErrCancelled", err)
		}
		if m.Installed(dir) {
			t.Error("a cancelled install left a model")
		}
	})

	// A download that stopped half way carries on from what arrived, with
	// a range, and the checksum is still of the whole archive.
	t.Run("a second try carries on from what arrived", func(t *testing.T) {
		dir := t.TempDir()
		half := len(good) / 2
		if err := os.WriteFile(filepath.Join(dir, name+".part"), good[:half], 0o644); err != nil {
			t.Fatal(err)
		}
		var asked string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked = r.Header.Get("Range")
			var from int
			if _, err := fmt.Sscanf(asked, "bytes=%d-", &from); err != nil {
				_, _ = w.Write(good)
				return
			}
			w.Header().Set("content-length", strconv.Itoa(len(good)-from))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(good[from:])
		}))
		defer s.Close()
		m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good)), SHA256: sha(good)}
		if err := (Installer{}).Install(context.Background(), m, dir); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("bytes=%d-", half); asked != want {
			t.Errorf("asked for %q, want %q", asked, want)
		}
		if !m.Installed(dir) {
			t.Error("the model is not there after carrying on")
		}
	})
}

// How far a download has come is said as it goes, about once a second by
// the clock it is handed, and nothing is said once it is over.
//
// On the real disk: the download is written into the models folder.
func TestADownloadSaysHowFarItHasCome(t *testing.T) {
	const name = "test-model"
	good := modelArchive(t, map[string]string{name + "/tokens.txt": "a 0\n"})
	s := serving(t, good)
	// Every reading of the clock is two seconds after the one before, so
	// every read of the body is a report.
	var mu sync.Mutex
	at := time.Unix(0, 0)
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		at = at.Add(2 * time.Second)
		return at
	}
	var said []*Progress
	in := Installer{Now: now, Report: func(p *Progress) { said = append(said, p) }}
	m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good))}
	if err := in.Install(context.Background(), m, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var fetching, unpacking bool
	for _, p := range said {
		if p == nil {
			continue
		}
		if strings.HasPrefix(p.Text, "fetching ") && p.Fraction > 0 {
			fetching = true
		}
		if p.Text == "unpacking" {
			unpacking = true
		}
	}
	if !fetching || !unpacking {
		t.Errorf("said %v, want how far it is fetched and the unpacking", said)
	}
	if said[len(said)-1] != nil {
		t.Errorf("the last word was %+v, want nothing once it is over", said[len(said)-1])
	}
}

// A clock whose every reading is two seconds after the one before, so
// every piece written is a report.
func ticking() func() time.Time {
	var mu sync.Mutex
	at := time.Unix(0, 0)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		at = at.Add(2 * time.Second)
		return at
	}
}

// An archive with one file of n bytes in it, the way a model is mostly
// its one large file, and how much it unpacks to. The large file comes
// first, so a stop heard in it is heard inside the file and never
// between two files.
func bigArchive(t *testing.T, name string, n int) ([]byte, int64) {
	t.Helper()
	body := strings.Repeat("0123456789abcdef", n/16)
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	for _, f := range []struct{ name, body string }{{name + "/model.onnx", body}, {name + "/tokens.txt", "a 0\n"}} {
		if err := w.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bzip(t, plain.Bytes()), int64(len("a 0\n") + len(body))
}

// Unpacking says how far it has come too, against what the model is known
// to take once unpacked, with the time left at the pace kept so far. Tim
// saw only "Unpacking" for over a minute, testing #24 on 17 November.
//
// On the real disk: the model is unpacked into the models folder.
func TestUnpackingSaysHowFarItHasCome(t *testing.T) {
	const name = "test-model"
	good, unpacked := bigArchive(t, name, 512*1024)
	s := serving(t, good)
	var said []Progress
	in := Installer{Now: ticking(), Report: func(p *Progress) {
		if p != nil && p.Text == "unpacking" {
			said = append(said, *p)
		}
	}}
	m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good)), Unpacked: unpacked}
	if err := in.Install(context.Background(), m, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(said) < 3 {
		t.Fatalf("said %+v while unpacking, want how far it came as it went", said)
	}
	if said[0].Fraction != 0 {
		t.Errorf("began at %v, want 0", said[0].Fraction)
	}
	between := false
	for i, p := range said[1:] {
		if p.Fraction < said[i].Fraction {
			t.Errorf("went back from %v to %v", said[i].Fraction, p.Fraction)
		}
		if p.Fraction > 0 && p.Fraction < 1 {
			between = true
			// Each reading is two seconds on, so what is done came at a
			// pace of done over the seconds since it began.
			done := p.Fraction * float64(unpacked)
			rate := done / (2 * float64(i+1))
			if want := (float64(unpacked) - done) / rate; math.Abs(p.Remaining-want) > 0.01 {
				t.Errorf("at %v said %.2f s left, want %.2f", p.Fraction, p.Remaining, want)
			}
		}
	}
	if !between {
		t.Errorf("said %+v, want a share between none and all", said)
	}
	if last := said[len(said)-1]; last.Fraction != 1 {
		t.Errorf("ended at %v, want 1", last.Fraction)
	}
}

// Cancel is heard in the middle of the model's one large file, not only
// once it is written, and nothing half unpacked is left. The archive had
// arrived whole, so Download unpacks it again without fetching it again.
//
// On the real disk: the model is unpacked into the models folder.
func TestCancelIsHeardWhileUnpacking(t *testing.T) {
	const name = "test-model"
	good, unpacked := bigArchive(t, name, 4<<20)
	var mu sync.Mutex
	asked := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked++
		mu.Unlock()
		w.Header().Set("content-length", strconv.Itoa(len(good)))
		_, _ = w.Write(good)
	}))
	t.Cleanup(s.Close)
	sum := sha256.Sum256(good)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	furthest := 0.0
	in := Installer{Now: ticking(), Report: func(p *Progress) {
		if p == nil || p.Text != "unpacking" {
			return
		}
		furthest = max(furthest, p.Fraction)
		if p.Fraction > 0 {
			cancel()
		}
	}}
	dir := t.TempDir()
	m := Model{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good)), Unpacked: unpacked, SHA256: hex.EncodeToString(sum[:])}
	if err := in.Install(ctx, m, dir); !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v, want it cancelled", err)
	}
	if furthest == 0 || furthest >= 0.5 {
		t.Errorf("got to %v before it stopped, want it stopped early in the file", furthest)
	}
	if m.Installed(dir) {
		t.Error("a cancelled unpacking left a model")
	}
	if _, err := os.Stat(filepath.Join(dir, name+".unpacking")); err == nil {
		t.Error("the half unpacked folder was left behind")
	}

	if err := (Installer{}).Install(context.Background(), m, dir); err != nil {
		t.Fatal(err)
	}
	if !m.Installed(dir) {
		t.Error("the second Download did not unpack the model")
	}
	mu.Lock()
	defer mu.Unlock()
	if asked != 1 {
		t.Errorf("fetched %d times, want once: the archive had arrived whole", asked)
	}
}

// A tar can name anything it likes, including a way out of the folder it is
// being written into. Nothing from a network gets to choose where it lands.
//
// On the real disk: what is tested is that nothing is written outside a folder on the disk.
func TestAnArchiveCannotWriteOutsideWhereItIsPut(t *testing.T) {
	root := t.TempDir()
	into := filepath.Join(root, "into")
	guard := filepath.Join(root, "guard.txt")
	if err := os.WriteFile(guard, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "absolute-should-never-exist")
	for _, name := range []string{"../guard.txt", "../../guard.txt", "a/../../guard.txt", outside} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "a.tar.bz2")
			if err := os.WriteFile(archive, modelArchive(t, map[string]string{name: "got out"}), 0o644); err != nil {
				t.Fatal(err)
			}
			_ = unpackTarBz2(context.Background(), archive, into, nil)
			body, _ := os.ReadFile(guard)
			if string(body) != "untouched" {
				t.Fatalf("%s wrote over a file outside the folder", name)
			}
			if _, err := os.Stat(outside); err == nil {
				t.Fatalf("%s wrote an absolute path", name)
			}
		})
	}
}

// A link is a way to point at something outside the folder without naming
// it in the header. The models have none, so none is written.
//
// On the real disk: what is tested is that no link is written on the disk.
func TestLinksInAnArchiveAreNotWritten(t *testing.T) {
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	for _, h := range []*tar.Header{
		{Name: "tokens.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg},
		{Name: "escape", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "escape2", Mode: 0o777, Typeflag: tar.TypeLink, Linkname: "tokens.txt"},
	} {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			_, _ = w.Write([]byte("ok"))
		}
	}
	_ = w.Close()
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.bz2")
	if err := os.WriteFile(archive, bzip(t, plain.Bytes()), 0o644); err != nil {
		t.Fatal(err)
	}
	into := filepath.Join(dir, "into")
	if err := unpackTarBz2(context.Background(), archive, into, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(into, "escape")); err == nil {
		t.Error("a symlink was written")
	}
	if _, err := os.Lstat(filepath.Join(into, "escape2")); err == nil {
		t.Error("a hard link was written")
	}
	if _, err := os.Stat(filepath.Join(into, "tokens.txt")); err != nil {
		t.Error("the ordinary file was thrown away with them")
	}
}

// The list is what the app offers, so it has to be offerable: every entry
// needs the words Tim reads before agreeing to a download, and exactly one
// is the one to reach for when nobody has chosen.
func TestEverySpeechModelCanBeOffered(t *testing.T) {
	models := Models()
	if len(models) == 0 {
		t.Fatal("no speech models at all")
	}
	recommended := 0
	seen := map[string]bool{}
	for _, m := range models {
		if m.Name == "" || m.Title == "" || m.About == "" || m.Languages == "" || m.Maker == "" {
			t.Errorf("%q cannot be shown: %+v", m.Name, m)
		}
		if !strings.HasPrefix(m.URL, "https://") {
			t.Errorf("%s is not fetched over https: %q", m.Name, m.URL)
		}
		if m.Download <= 0 || m.Unpacked <= 0 {
			t.Errorf("%s does not say how big it is", m.Name)
		}
		// Untrusted input. A model with no checksum is a model nothing
		// checks, and it is unpacked straight onto the Mac.
		if len(m.SHA256) != 64 {
			t.Errorf("%s has no usable checksum: %q", m.Name, m.SHA256)
		}
		if seen[m.Name] {
			t.Errorf("%s is listed twice", m.Name)
		}
		seen[m.Name] = true
		if m.Recommended {
			recommended++
		}
		if found, ok := ByName(Models(), m.Name); !ok || found.Name != m.Name {
			t.Errorf("%s cannot be found by name", m.Name)
		}
	}
	if recommended != 1 {
		t.Errorf("%d models are recommended, want exactly 1", recommended)
	}
	if _, ok := ByName(Models(), "no-such-model"); ok {
		t.Error("found a model that does not exist")
	}
}

// A speech model folder is ready once its tokens.txt is there and not
// empty. An empty file, which a download cut short leaves, is no model.
//
// On the real disk: the speech library loads the model folder, so it is looked at on the disk.
func TestASpeechModelIsReadyByItsTokens(t *testing.T) {
	dir := t.TempDir()
	if Ready(dir) {
		t.Error("an empty folder is ready")
	}
	tokens := filepath.Join(dir, "tokens.txt")
	if err := os.WriteFile(tokens, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if Ready(dir) {
		t.Error("an empty tokens.txt is ready")
	}
	if err := os.WriteFile(tokens, []byte("a 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Ready(dir) {
		t.Error("a model with its tokens is not ready")
	}
}

// Removing a model takes its folder and whatever a download left, and
// nothing else in the models folder.
//
// On the real disk: what is removed is a folder on the disk.
func TestRemovingASpeechModelLeavesTheRest(t *testing.T) {
	dir := t.TempDir()
	m := Models()[0]
	for _, p := range []string{m.Name + "/tokens.txt", m.Name + ".part", m.Name + ".unpacking/x", "other/tokens.txt"} {
		path := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("a 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := InUse(Models(), dir); !ok || got.Name != m.Name {
		t.Fatalf("in use is %q, want %q", got.Name, m.Name)
	}
	if err := Remove(m, dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{m.Name, m.Name + ".part", m.Name + ".unpacking"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t.Errorf("%s was left", p)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "other/tokens.txt")); err != nil {
		t.Error("something else in the models folder went too")
	}
	if _, ok := InUse(Models(), dir); ok {
		t.Error("a model is in use after it was removed")
	}
	if err := Remove(Model{Name: ".."}, dir); err == nil {
		t.Error("removed outside the models folder")
	}
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
