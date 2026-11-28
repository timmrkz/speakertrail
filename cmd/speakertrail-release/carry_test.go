package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dsnet/compress/bzip2"
)

// entry is one thing in a test archive. A name that ends in / is a
// folder, and link makes it a symbolic link to there.
type entry struct {
	name, body, link string
}

// tarBzip2 packs entries the way sherpa-onnx packs its release.
func tarBzip2(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	bz, err := bzip2.NewWriter(&buf, nil)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(bz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o755, Typeflag: tar.TypeReg, Size: int64(len(e.body))}
		switch {
		case e.link != "":
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, e.link, 0
		case strings.HasSuffix(e.name, "/"):
			h.Typeflag, h.Size = tar.TypeDir, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

const testRelease = "sherpa-onnx-v2.0.1-osx-arm64-shared-no-tts-lib"

// theRelease is an archive like sherpa-onnx's for a Mac, with lib as
// the library read back.
func theRelease(t *testing.T, lib string) []byte {
	return tarBzip2(t,
		entry{name: testRelease + "/"},
		entry{name: testRelease + "/lib/"},
		entry{name: testRelease + "/lib/libsherpa-onnx-c-api.dylib", body: lib},
		entry{name: testRelease + "/lib/libonnxruntime.dylib", body: "onnxruntime"},
	)
}

// server hands out one archive, and counts how often it was asked, and
// fails as many times as it is told to first.
type server struct {
	mu      sync.Mutex
	archive []byte
	asked   []string
	failing int
}

// get carries on from what path holds, as fetch.Getter does.
func (s *server) get(url string, expect int64, path, _ string) error {
	s.mu.Lock()
	s.asked = append(s.asked, url)
	failing := s.failing > 0
	s.failing--
	s.mu.Unlock()
	if expect != int64(len(s.archive)) {
		return errors.New("asked for the wrong size")
	}
	if failing {
		have, _ := os.ReadFile(path)
		n := min(len(have)+4, len(s.archive))
		_ = os.WriteFile(path, s.archive[:n], 0o644)
		return errors.New("connection reset")
	}
	return os.WriteFile(path, s.archive, 0o644)
}

// pinOf is the pin of an archive, its sum and size.
func pinOf(archive []byte) speechPin {
	return speechPin{"osx-arm64", hexSum(archive), "libsherpa-onnx-c-api.dylib", int64(len(archive))}
}

// The speech library without espeak-ng is fetched once, from
// sherpa-onnx's release of the module's version, and every build after
// finds it unpacked and unchanged without asking again.
//
// On the real disk: the library lives in .build/speech between builds.
func TestTheSpeechLibraryIsFetchedOnce(t *testing.T) {
	t.Parallel()
	archive := theRelease(t, "speech")
	s := &server{archive: archive}
	pin := pinOf(archive)
	dir := t.TempDir()
	var said bytes.Buffer
	got, err := speechLibs(dir, "v2.0.1", pin, s.get, &said)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, testRelease, "lib"); got != want {
		t.Errorf("libraries in %s, want %s", got, want)
	}
	if want := []string{"https://github.com/k2-fsa/sherpa-onnx/releases/download/v2.0.1/" + testRelease + ".tar.bz2"}; !slices.Equal(s.asked, want) {
		t.Errorf("asked %q, want %q", s.asked, want)
	}
	if !strings.Contains(said.String(), "Fetching the speech library") {
		t.Errorf("a fetch said %q", said.String())
	}
	if b, err := os.ReadFile(filepath.Join(got, "libonnxruntime.dylib")); err != nil || string(b) != "onnxruntime" {
		t.Errorf("unpacked %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, testRelease+".tar.bz2.part")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the part of the download is left: %v", err)
	}

	said.Reset()
	if again, err := speechLibs(dir, "v2.0.1", pin, s.get, &said); err != nil || again != got {
		t.Errorf("the second build got %s, %v", again, err)
	}
	if len(s.asked) != 1 || said.Len() != 0 {
		t.Errorf("the second build fetched again, asked %q, said %q", s.asked, said.String())
	}
}

// A library changed after it was unpacked never goes into a build: it is
// unpacked again from the archive, which is checked first, and not
// fetched again.
func TestAChangedSpeechLibraryIsUnpackedAgain(t *testing.T) {
	t.Parallel()
	archive := theRelease(t, "speech")
	s := &server{archive: archive}
	pin := pinOf(archive)
	dir := t.TempDir()
	got, err := speechLibs(dir, "v2.0.1", pin, s.get, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(got, "libsherpa-onnx-c-api.dylib")
	if err := os.WriteFile(lib, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := speechLibs(dir, "v2.0.1", pin, s.get, io.Discard); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(lib); string(b) != "speech" {
		t.Errorf("the library is %q, want it unpacked again", b)
	}
	if len(s.asked) != 1 {
		t.Errorf("fetched %d times, want once", len(s.asked))
	}
}

// What is not the archive that was checked, or holds what it should not,
// never reaches a build, and nothing of it is kept for the next.
func TestASpeechLibraryNotCheckedIsRefused(t *testing.T) {
	t.Parallel()
	good := theRelease(t, "speech")
	for _, c := range []struct {
		name    string
		archive []byte
		sum     string
		failing int
		want    string
	}{
		{"another file", good, hexSum([]byte("other")), 0, "is not the file that was checked"},
		{"still carrying espeak-ng", theRelease(t, "speech espeak-ng speech"), "", 0, "still carries espeak-ng"},
		{"without the library", tarBzip2(t, entry{name: testRelease + "/lib/libonnxruntime.dylib", body: "x"}), "", 0, "holds no libsherpa-onnx-c-api.dylib"},
		{"with a file outside its folder", tarBzip2(t, entry{name: "../evil", body: "x"}), "", 0, "refusing to write outside"},
		{"with the library in another folder", tarBzip2(t, entry{name: "other/lib/libsherpa-onnx-c-api.dylib", body: "x"}), "", 0, "holds no libsherpa-onnx-c-api.dylib"},
		{"with the library as a link", tarBzip2(t, entry{name: testRelease + "/lib/libsherpa-onnx-c-api.dylib", link: "/etc/passwd"}), "", 0, "holds no libsherpa-onnx-c-api.dylib"},
		{"never fetched", good, "", 3, "connection reset"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sum := c.sum
			if sum == "" {
				sum = hexSum(c.archive)
			}
			s := &server{archive: c.archive, failing: c.failing}
			dir := t.TempDir()
			_, err := speechLibs(dir, "v2.0.1", speechPin{"osx-arm64", sum, "libsherpa-onnx-c-api.dylib", int64(len(c.archive))}, s.get, io.Discard)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
			if _, err := os.Stat(filepath.Join(dir, testRelease+".ok")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a stamp was written: %v", err)
			}
		})
	}
}

// A download that breaks off is tried again, three times in all, as
// curl --retry 2 did, each carrying on from what came before.
func TestABrokenDownloadIsTriedAgain(t *testing.T) {
	t.Parallel()
	archive := theRelease(t, "speech")
	s := &server{archive: archive, failing: 2}
	pin := pinOf(archive)
	if _, err := speechLibs(t.TempDir(), "v2.0.1", pin, s.get, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(s.asked) != 3 {
		t.Errorf("asked %d times, want 3", len(s.asked))
	}
}

// recorder records what carryLibs asks of the Mac's tools, and fails the
// ones named in failing.
type recorder struct {
	mu      sync.Mutex
	ran     []string
	failing []string
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line := name + " " + strings.Join(args, " ")
	r.ran = append(r.ran, line)
	for _, f := range r.failing {
		if strings.HasPrefix(line, f) {
			return nil, errors.New(line + ": failed")
		}
	}
	return nil, nil
}

// The libraries to ship go beside the program in place of any carried
// before, writable, and the program is pointed at them and away from the
// module cache, with every rewrite signed again.
//
// On the real disk: the libraries are copied, and the tools that rewrite
// them are recorded.
func TestTheSpeechLibraryIsCarriedBesideTheProgram(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	ship := filepath.Join(top, "ship")
	bin := filepath.Join(top, "bin")
	libdir := filepath.Join(bin, "lib")
	program := filepath.Join(bin, "speakertrail-app")
	from := "/go/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-macos/v2@v2.0.1/lib/aarch64-apple-darwin"
	for name, body := range map[string]string{
		ship + "/libsherpa-onnx-c-api.dylib": "c-api",
		ship + "/libonnxruntime.dylib":       "onnx",
		libdir + "/libsherpa-onnx-old.dylib": "old",
		libdir + "/notes.txt":                "kept",
		program:                              "program",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The module cache's files cannot be written.
	for _, n := range []string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"} {
		if err := os.Chmod(filepath.Join(ship, n), 0o444); err != nil {
			t.Fatal(err)
		}
	}

	r := &recorder{failing: []string{"install_name_tool -add_rpath @executable_path "}}
	if err := carryLibs(program, libdir, from, ship, r.run); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(libdir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		info, _ := e.Info()
		if info.Mode().Perm()&0o200 == 0 {
			t.Errorf("%s cannot be written", e.Name())
		}
	}
	if want := []string{"libonnxruntime.dylib", "libsherpa-onnx-c-api.dylib", "notes.txt"}; !slices.Equal(names, want) {
		t.Errorf("lib holds %q, want %q", names, want)
	}
	for _, want := range []string{
		"strip -x " + libdir + "/libonnxruntime.dylib",
		"strip -x " + libdir + "/libsherpa-onnx-c-api.dylib",
		"install_name_tool -id @rpath/libonnxruntime.dylib " + libdir + "/libonnxruntime.dylib",
		"install_name_tool -change " + from + "/libonnxruntime.dylib @rpath/libonnxruntime.dylib " + program,
		"install_name_tool -add_rpath @executable_path/../Frameworks " + program,
		"install_name_tool -add_rpath @executable_path/lib " + program,
		"install_name_tool -delete_rpath " + from + " " + program,
		"codesign -s - -f " + program,
		"codesign -s - -f " + libdir + "/libsherpa-onnx-c-api.dylib",
	} {
		if !slices.Contains(r.ran, want) {
			t.Errorf("never ran %q", want)
		}
	}
	// The rpath goes before the signature, which it would break.
	if slices.Index(r.ran, "install_name_tool -delete_rpath "+from+" "+program) > slices.Index(r.ran, "codesign -s - -f "+program) {
		t.Error("the program was signed before it was rewritten")
	}
}

// Stripping is the one step whose failure stops the build, as set -e had
// it in the script: what is left would be shipped with every local
// symbol.
func TestALibraryThatCannotBeStrippedStopsTheBuild(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	ship := filepath.Join(top, "ship")
	if err := os.MkdirAll(ship, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ship, "libonnxruntime.dylib"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &recorder{failing: []string{"strip"}}
	err := carryLibs(filepath.Join(top, "speakertrail-app"), filepath.Join(top, "lib"), "/from", ship, r.run)
	if err == nil || !strings.Contains(err.Error(), "strip -x") {
		t.Errorf("got %v", err)
	}
}

// A program is read back for whether it runs on another Mac: each library
// it links as @rpath is found under a place it looks, and none of those
// places is the module cache of the Mac that built it.
//
// On the real disk: the program and its libraries are files in a bundle.
func TestAProgramIsReadBackForAnotherMac(t *testing.T) {
	t.Parallel()
	const gomod = "/Users/runner/go/pkg/mod"
	speech := []string{"/usr/lib/libSystem.B.dylib", "@rpath/libsherpa-onnx-c-api.dylib", "@rpath/libonnxruntime.dylib"}
	for _, c := range []struct {
		name   string
		data   []byte
		frames []string
		want   []string
	}{
		{"as carried", program{links: speech, rpaths: []string{"@executable_path", "@executable_path/../Frameworks"}}.bytes(),
			[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"}, nil},
		{"a library missing", program{links: speech, rpaths: []string{"@executable_path/../Frameworks"}}.bytes(),
			[]string{"libsherpa-onnx-c-api.dylib"}, []string{"needs libonnxruntime.dylib and cannot find it"}},
		{"looking nowhere", program{links: speech}.bytes(),
			[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"},
			[]string{"needs libonnxruntime.dylib and cannot find it", "needs libsherpa-onnx-c-api.dylib and cannot find it"}},
		{"still in the module cache", program{links: speech, rpaths: []string{"@executable_path/../Frameworks", gomod + "/github.com/k2-fsa/lib"}}.bytes(),
			[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"},
			[]string{"still looks in the build machine's module cache, which exists on no other machine: " + gomod + "/github.com/k2-fsa/lib"}},
		{"linked weakly", program{links: []string{"/usr/lib/libSystem.B.dylib"}, other: []load{{cmd: loadWeakDylib, name: "@rpath/libonnxruntime.dylib"}}, rpaths: []string{"@loader_path/../Frameworks"}}.bytes(),
			nil, []string{"needs libonnxruntime.dylib and cannot find it"}},
		{"universal, one architecture short", fat(
			program{links: speech, rpaths: []string{"@executable_path/../Frameworks"}},
			program{cpu: 0x01000007, links: speech}),
			[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"},
			[]string{"needs libonnxruntime.dylib and cannot find it", "needs libsherpa-onnx-c-api.dylib and cannot find it"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			app := filepath.Join(t.TempDir(), "Speaker Trail.app", "Contents")
			exe := filepath.Join(app, "MacOS", "speakertrail-app")
			files := map[string][]byte{exe: c.data}
			for _, f := range c.frames {
				files[filepath.Join(app, "Frameworks", f)] = []byte("library")
			}
			for name, data := range files {
				if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, data, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got, err := verifyLibs(exe, gomod)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("found %q, want %q", got, c.want)
			}
			if err := verified(exe, gomod); (err == nil) != (c.want == nil) {
				t.Errorf("verified said %v", err)
			}
		})
	}
}

// A file that is no program cannot be read back, and says so rather than
// passing.
func TestWhatIsNoProgramIsNotPassed(t *testing.T) {
	t.Parallel()
	name := filepath.Join(t.TempDir(), "speakertrail-app")
	if err := os.WriteFile(name, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verified(name, "/go/pkg/mod"); err == nil {
		t.Error("a shell script passed as a program")
	}
}
