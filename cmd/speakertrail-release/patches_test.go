package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// programOf is the app's program with code of its own after it, as hard to
// pack as real code, so a piece is large and a patch between two builds is
// small.
func programOf(code []byte) []byte { return append(bundled.bytes(), code...) }

// code is what a build compiles to, as random as compiled code.
var code = func() []byte {
	b := make([]byte, 96<<10)
	_, _ = rand.NewChaCha8([32]byte{1}).Read(b)
	return b
}()

// changed is the code after a change: a few bytes in, and every byte after
// them moved along.
var changed = append(append(append([]byte{}, code[:5000]...), "a new line of code"...), code[5000:]...)

// published is the folder pack wrote, under the names publish.yml uploads
// it as, for a zip of this name.
//
// On the real disk: pack leaves the build as files, and the workflow uploads them.
func published(t *testing.T, dir, zipName string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		switch name := e.Name(); {
		case name == partsFile:
			files[updates.PartsName(zipName)] = data
		case strings.HasPrefix(name, "piece-"), strings.HasPrefix(name, "patch-"):
			files[name] = data
		}
	}
	return files
}

// releaseServing serves files as the release dev does.
func releaseServing(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[strings.TrimPrefix(r.URL.Path, "/dev/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// basesFrom is pack's bases, read from the release srv serves.
func basesFrom(srv *httptest.Server, names []string, log io.Writer) bases {
	b := bases{from: srv.URL + "/dev/", client: srv.Client(), most: patchesPerBuild}
	for _, name := range names {
		if p, ok := b.readParts([]string{name}, log)[name]; ok {
			b.parts = append(b.parts, p)
		}
	}
	return b
}

// A build is packed with a patch of each file that changed against the
// same file of the builds the release holds, which makes the file the
// app's own code makes from it, weighs less than the piece, and is named
// in the parts by the file it starts from. A file that did not change has
// none, and the build signs with them.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestABuildIsPackedWithPatchesAgainstTheBuildsListed(t *testing.T) {
	release(t)
	app := "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	srv := releaseServing(t, published(t, first, "app-main-1.zip"))

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := basesFrom(srv, []string{"app-main-1.parts.json"}, &log)
	if err := pack(in, second, packer, b, &log); err != nil {
		t.Fatal(err)
	}
	p := partsIn(t, second)
	for _, part := range p.Parts {
		switch {
		case part.Bytes != nil:
		case part.Name == app:
			old := programOf(code)
			if len(part.Patches) != 1 || part.Patches[0].From != sum(old) {
				t.Fatalf("the program has the patches %+v, want one from the program before", part.Patches)
			}
			pt := part.Patches[0]
			if pt.Size >= part.Packed {
				t.Errorf("the patch is %d bytes, the piece %d", pt.Size, part.Packed)
			}
			patch, err := os.ReadFile(filepath.Join(second, updates.PatchName(pt.From, part.SHA256)))
			if err != nil || int64(len(patch)) != pt.Size {
				t.Fatalf("the patch is %d bytes, %v", len(patch), err)
			}
			made, err := updates.ApplyPatch(old, patch, part.Size)
			if err != nil || !bytes.Equal(made, programOf(changed)) {
				t.Errorf("the patch does not make the program: %v", err)
			}
		default:
			if part.Patches != nil {
				t.Errorf("%s did not change and has the patches %+v", part.Name, part.Patches)
			}
		}
	}
	if err := signBuild(in, "main", "main", "0.1.0-main.def5678", "def5678abc12",
		srv.URL+"/dev/app-main-2.zip", filepath.Join(second, partsFile), filepath.Join(t.TempDir(), "entry.json")); err != nil {
		t.Errorf("a build with patches was not signed: %v\n%s", err, log.String())
	}
}

// What cannot be had of a build patched against, its pieces, or its parts
// when they are gone, no parts or say another build, costs only its
// patches, never the publish, and the run is warned.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestWhatCannotBeHadToPatchAgainstCostsOnlyThePatches(t *testing.T) {
	release(t)
	app := "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	// The parts are there, the pieces they name are gone.
	served := map[string][]byte{}
	for name, data := range published(t, first, "app-main-1.zip") {
		if strings.HasSuffix(name, ".parts.json") {
			served[name] = data
		}
	}
	served["app-pr-2.parts.json"] = []byte("{")
	p := partsIn(t, first)
	p.Size++
	served["app-pr-3.parts.json"] = mustMarshal(t, p)
	srv := releaseServing(t, served)

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := basesFrom(srv, []string{"app-main-1.parts.json", "app-pr-2.parts.json", "app-pr-3.parts.json", "app-pr-4.parts.json"}, &log)
	if len(b.parts) != 1 {
		t.Fatalf("%d parts were read, want the one that is parts\n%s", len(b.parts), log.String())
	}
	for _, name := range []string{"app-pr-2", "app-pr-3", "app-pr-4"} {
		if !strings.Contains(log.String(), "::warning::"+name+".parts.json could not be read") {
			t.Errorf("the run is not warned that %s.parts.json could not be read:\n%s", name, log.String())
		}
	}
	if err := pack(in, out, packer, b, &log); err != nil {
		t.Fatal(err)
	}
	for _, part := range partsIn(t, out).Parts {
		if part.Patches != nil {
			t.Errorf("%s has the patches %+v from builds that cannot be had", part.Name, part.Patches)
		}
	}
	if !strings.Contains(log.String(), "no patch of "+app) {
		t.Errorf("the log does not say why the program has no patch:\n%s", log.String())
	}
}

// builtBefore publishes n builds of the app, each with a program of its
// own, as app-pr-1 to app-pr-n, and returns what the release serves and
// the program of each, in that order.
//
// On the real disk: pack writes each build as files, and the release serves them.
func builtBefore(t *testing.T, n int) (map[string][]byte, [][]byte) {
	t.Helper()
	app := "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	served := map[string][]byte{}
	var olds [][]byte
	for i := range n {
		files := theApp("main")
		old := programOf(append(append([]byte{}, code...), byte(i)))
		files[app] = old
		olds = append(olds, old)
		for name, data := range published(t, packed(t, files), fmt.Sprintf("app-pr-%d.zip", i+1)) {
			served[name] = data
		}
	}
	return served, olds
}

// patchesFrom are the files the patches in out start from, sorted.
func patchesFrom(t *testing.T, out string) []string {
	t.Helper()
	var got []string
	for _, part := range partsIn(t, out).Parts {
		for _, pt := range part.Patches {
			got = append(got, pt.From)
		}
	}
	slices.Sort(got)
	return got
}

// sums are the SHA-256 of each program, sorted.
func sums(programs ...[]byte) []string {
	var out []string
	for _, p := range programs {
		out = append(out, sum(p))
	}
	slices.Sort(out)
	return out
}

// A build has at most the patches it has room for, and those are from
// the builds patched against that come first. The release holds at most
// a thousand files, and patches from every build to every build filled
// it.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestABuildHasThePatchesFromTheBuildsFirstThatItHasRoomFor(t *testing.T) {
	release(t)
	const n = 8
	served, olds := builtBefore(t, n)
	srv := releaseServing(t, served)
	// The order they are patched against in, not the order they were made.
	order := []int{5, 2, 7, 0, 3, 6, 1, 4}
	var names []string
	var first [][]byte
	for _, i := range order {
		names = append(names, fmt.Sprintf("app-pr-%d.parts.json", i+1))
		first = append(first, olds[i])
	}

	files := theApp("main")
	files["Speaker Trail.app/Contents/MacOS/speakertrail-app"] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, most := range []int{0, 1, 3, patchesPerBuild} {
		want := min(most, n)
		out := filepath.Join(dir, fmt.Sprint("packed-", most))
		var log bytes.Buffer
		b := basesFrom(srv, names, &log)
		b.most = most
		if err := pack(in, out, packer, b, &log); err != nil {
			t.Fatal(err)
		}
		if got := patchesFrom(t, out); !slices.Equal(got, sums(first[:want]...)) {
			t.Errorf("with room for %d, the patches are from %.12v, want from the first %d", most, got, want)
		}
		patches, _ := filepath.Glob(filepath.Join(out, "patch-*"))
		if len(patches) != want {
			t.Errorf("with room for %d, %d patches were written, want %d", most, len(patches), want)
		}
	}
}

// pack, as publish.yml runs it, reads the channel list and the release's
// files, has room for fewer patches once the release is nearly full, and
// patches against the channel's own build before first, though it is the
// oldest: the app that follows the channel is the one that updates.
//
// On the real disk: the workflow writes the list and the release's files, and the release tool reads them and writes the packed build.
func TestPackPatchesFromTheChannelsBuildFirstOnANearlyFullRelease(t *testing.T) {
	release(t)
	served, olds := builtBefore(t, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := served[strings.TrimPrefix(r.URL.Path, "/dev/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	list := `{"channels":[` +
		`{"channel":"pr-1","url":"https://github.com/timmrkz/speakertrail/releases/download/dev/app-pr-1.zip"},` +
		`{"channel":"pr-2","url":"https://github.com/timmrkz/speakertrail/releases/download/dev/app-pr-2.zip"},` +
		`{"channel":"pr-3","url":"https://github.com/timmrkz/speakertrail/releases/download/dev/app-pr-3.zip"}]}`
	// pr-1 is the oldest build, and the release holds 799 files, which
	// leaves room for a single patch.
	var assets strings.Builder
	assets.WriteString(`{"assets":[`)
	at := now.Add(-3 * time.Hour)
	for i := range 3 {
		fmt.Fprintf(&assets, `{"name":"app-pr-%d.zip","createdAt":%q},{"name":"app-pr-%d.parts.json","createdAt":%q},`,
			i+1, at.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), i+1, at.Add(time.Duration(i)*time.Hour).Format(time.RFC3339))
	}
	for i := range maxAssets - keepFree - 1 - 7 {
		fmt.Fprintf(&assets, `{"name":"piece-%d.zst","createdAt":%q},`, i, at.Format(time.RFC3339))
	}
	assets.WriteString(`{"name":"channels.json","createdAt":"2026-11-21T12:00:00Z"}]}`)
	listPath, assetsPath := filepath.Join(dir, "channels.json"), filepath.Join(dir, "assets.json")
	if err := os.WriteFile(listPath, []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetsPath, []byte(assets.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	files := theApp("main")
	files["Speaker Trail.app/Contents/MacOS/speakertrail-app"] = programOf(changed)
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")

	if err := packCommand([]string{"-zip", in, "-out", out, "-from", srv.URL + "/dev/",
		"-channel", "pr-1", "-list", listPath, "-assets", assetsPath}); err != nil {
		t.Fatal(err)
	}
	if got, want := patchesFrom(t, out), sums(olds[0]); !slices.Equal(got, want) {
		t.Errorf("the patches are from %.12v, want only from pr-1's build, %.12v", got, want)
	}
}

func sum(b []byte) string {
	d, _, _ := updates.Digest(bytes.NewReader(b))
	return hex.EncodeToString(d)
}
