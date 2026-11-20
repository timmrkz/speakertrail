package main

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// releaseServing serves the folder pack wrote as the release dev does,
// under the names publish.yml uploads it as, for a zip of this name.
//
// On the real disk: pack leaves the build as files, and the release serves them.
func releaseServing(t *testing.T, dir, zipName string) *httptest.Server {
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
		case strings.HasPrefix(name, "piece-"):
			files[name] = data
		}
	}
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

// A build is packed with a patch of each file that changed against the
// same file of the builds the release holds, which makes the file the
// app's own code makes from it, weighs less than the piece, and is named
// in the parts by the file it starts from. A file that did not change has
// none, and the build signs with them.
//
// On the real disk: the release tool reads the zip and the entries, and writes the packed build, as files.
func TestABuildIsPackedWithPatchesAgainstTheBuildsListed(t *testing.T) {
	release(t)
	app := "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	srv := releaseServing(t, first, "app-main-1.zip")

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := bases{parts: readBases([]string{filepath.Join(first, partsFile)}, &log), from: srv.URL + "/dev/", client: srv.Client()}
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
// when they are no parts or say another build, costs only its patches,
// never the publish.
//
// On the real disk: the release tool reads the zip and the parts, and writes the packed build, as files.
func TestWhatCannotBeHadToPatchAgainstCostsOnlyThePatches(t *testing.T) {
	release(t)
	app := "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	gone := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(gone.Close)
	notParts := filepath.Join(t.TempDir(), "not-parts.json")
	if err := os.WriteFile(notParts, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := partsIn(t, first)
	p.Size++
	otherSize := filepath.Join(t.TempDir(), "other-size.json")
	if err := os.WriteFile(otherSize, mustMarshal(t, p), 0o644); err != nil {
		t.Fatal(err)
	}

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")
	var log bytes.Buffer
	read := readBases([]string{filepath.Join(first, partsFile), notParts, otherSize}, &log)
	if len(read) != 1 {
		t.Fatalf("%d parts were read, want the one that is parts\n%s", len(read), log.String())
	}
	b := bases{parts: read, from: gone.URL + "/dev/", client: gone.Client()}
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

func sum(b []byte) string {
	d, _, _ := updates.Digest(bytes.NewReader(b))
	return hex.EncodeToString(d)
}
