package main

// carry-libs carries the speech library next to the app's program and
// points the program at it. It was scripts/carry-libs.sh and
// scripts/speech-libs.sh, ported from FrameFairy's, and moved here with
// #57. make runs it on a Mac right after it builds the program.
//
// sherpa-onnx ships shared libraries rather than an archive, and its cgo
// directive writes the Go module cache path into the program as the place
// to find them:
//
//	RUNPATH /Users/tim/go/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-macos/v2@v2.0.1/lib/...
//
// Nobody else has that folder, so the program dies before it draws
// anything. A build made on GitHub's Mac and installed from the Updates
// page is the everyday case here. See docs/BUILD.md.
//
// What goes beside the program is not the module's libraries, which
// carry espeak-ng, but the same release built without speech synthesis,
// see speechLibs. The program is linked against the module's as ever,
// and finds these instead, because they are what is under its rpath.
//
// The scripts also did this on Linux, with patchelf and ldd, but make
// only ever ran them on a Mac, so that half did not come along.

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"crypto/sha256"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// speechModule is the Go module whose libraries the program is linked
// against on a Mac.
const speechModule = "github.com/k2-fsa/sherpa-onnx-go-macos/v2"

// speechPin is one archive of sherpa-onnx's release built without speech
// synthesis: its platform in the archive's name, its SHA-256, and the
// library in it that is read back for espeak-ng.
type speechPin struct {
	platform, sum, lib string
}

// speechPins are the archives checked so far, by Go's name for the
// system. The sums are of the v2.0.1 archives, read on 28 November 2026
// and matching what speech-libs.sh had pinned.
var speechPins = map[string]speechPin{
	"darwin/arm64": {"osx-arm64", "ac5594ad237db56ec4a6c4b3fc48700441b33a07bb71c235f35530a52a93f2e3", "libsherpa-onnx-c-api.dylib"},
	"linux/amd64":  {"linux-x64", "ce437357915491e6f2754b52bec7dcf62ff6fc6aaed9cdcd66f17807bbaef374", "libsherpa-onnx-c-api.so"},
}

// speechLibs gives the folder holding the speech libraries to ship,
// fetching them into dir the first time.
//
// The sherpa-onnx Go module brings prebuilt libraries, and they have
// espeak-ng compiled into them, for speech synthesis. espeak-ng is GPL 3,
// and Speaker Trail never synthesises speech, so it ships none of it, the
// way FrameFairy ships none. sherpa-onnx publishes the same release
// built without speech synthesis, the -no-tts archives. There every
// synthesis function is still there and only says "TTS is not enabled",
// so the Go module works against them unchanged. Both were compared
// function by function, and the Go module calls nothing these do not
// have.
//
// The archive is pinned by its SHA-256, and what is inside is read back:
// a library that still carries espeak-ng is refused.
func speechLibs(dir, version string, pin speechPin, fetch func(url string, w io.Writer) error, say io.Writer) (string, error) {
	name := fmt.Sprintf("sherpa-onnx-%s-%s-shared-no-tts-lib", version, pin.platform)
	out := filepath.Join(dir, name, "lib")
	lib := filepath.Join(out, pin.lib)
	stamp := filepath.Join(dir, name+".ok")

	// The library unpacked is checked on every build against the SHA-256
	// it had then, which the stamp holds. One sum of one library is a
	// fraction of a second. It used to be a file test, so a library
	// changed after it was unpacked went into the program unseen. One
	// that no longer matches is unpacked again from the archive, which
	// is checked first.
	if want, err := os.ReadFile(stamp); err == nil {
		if got, err := sumOfFile(lib); err == nil && got == strings.TrimSpace(string(want)) {
			return out, nil
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(dir, name+".tar.bz2")
	if got, _ := sumOfFile(archive); got != pin.sum {
		fmt.Fprintf(say, "Fetching the speech library without speech synthesis, %s\n", name)
		url := fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/%s/%s.tar.bz2", version, name)
		if err := fetchTo(url, archive, fetch); err != nil {
			return "", err
		}
	}
	got, err := sumOfFile(archive)
	if err != nil {
		return "", err
	}
	if got != pin.sum {
		_ = os.Remove(archive)
		return "", fmt.Errorf("%s.tar.bz2 is not the file that was checked.\n  expected %s\n  got      %s", name, pin.sum, got)
	}

	if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
		return "", err
	}
	if err := untarBzip2(archive, dir, name); err != nil {
		return "", fmt.Errorf("%s.tar.bz2: %w", name, err)
	}
	data, err := os.ReadFile(lib)
	if err != nil {
		return "", fmt.Errorf("%s.tar.bz2 holds no %s", name, pin.lib)
	}
	// Read back rather than trusted by its name.
	if bytes.Contains(data, []byte("espeak-ng")) {
		return "", fmt.Errorf("%s still carries espeak-ng", lib)
	}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(stamp, []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		return "", err
	}
	return out, nil
}

// sumOfFile is the SHA-256 of a file, in hex.
func sumOfFile(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchTo downloads url to the file to, through a file beside it, so a
// download that breaks off leaves nothing under the name. It is tried
// three times, as curl --retry 2 did.
func fetchTo(url, to string, fetch func(url string, w io.Writer) error) error {
	part := to + ".part"
	var err error
	for range 3 {
		if err = fetchOnce(url, part, fetch); err == nil {
			return os.Rename(part, to)
		}
	}
	_ = os.Remove(part)
	return err
}

func fetchOnce(url, part string, fetch func(url string, w io.Writer) error) error {
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	err = fetch(url, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// fetchHTTP downloads url into w.
func fetchHTTP(url string, w io.Writer) error {
	client := http.Client{Timeout: 10 * time.Minute}
	r, err := client.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, r.Status)
	}
	_, err = io.Copy(w, r.Body)
	return err
}

// untarBzip2 unpacks the archive into dir. Everything in it has to be
// inside top, and be a folder or a file. The archive is pinned by its
// sum, so this never meets anything else, and is never asked to.
func untarBzip2(archive, dir, top string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	r := tar.NewReader(bzip2.NewReader(f))
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if !filepath.IsLocal(name) || (name != top && !strings.HasPrefix(name, top+string(filepath.Separator))) {
			return fmt.Errorf("it holds %s, outside %s", h.Name, top)
		}
		at := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(at, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(at, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0o755|0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, r)
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("it holds %s, which is neither a file nor a folder", h.Name)
		}
	}
}

// buildTools runs the Mac's own tools a build uses, by their full path,
// as macTools does for harden.
var buildTools = macRunner("a build", "strip", "install_name_tool", "codesign")

// carryLibs copies the libraries in ship to libdir and rewrites program
// and them to find each other there, wherever the program is moved. from
// is the module's folder, the one cgo pointed the program at.
func carryLibs(program, libdir, from, ship string, run func(name string, args ...string) ([]byte, error)) error {
	if err := os.MkdirAll(libdir, 0o755); err != nil {
		return err
	}
	old, err := os.ReadDir(libdir)
	if err != nil {
		return err
	}
	for _, e := range old {
		if strings.HasPrefix(e.Name(), "libsherpa-onnx-") || strings.HasPrefix(e.Name(), "libonnxruntime") {
			if err := os.RemoveAll(filepath.Join(libdir, e.Name())); err != nil {
				return err
			}
		}
	}
	shipped, err := os.ReadDir(ship)
	if err != nil {
		return err
	}
	for _, e := range shipped {
		if !e.Type().IsRegular() {
			continue
		}
		// Writable, since the module cache's are not and every one is
		// rewritten below.
		if err := copyFile(filepath.Join(ship, e.Name()), filepath.Join(libdir, e.Name()), 0o755); err != nil {
			return err
		}
	}
	libs, err := filepath.Glob(filepath.Join(libdir, "*.dylib"))
	if err != nil {
		return err
	}
	slices.Sort(libs)

	// Most of what follows is allowed to fail quietly, as it was in the
	// script, because most of it changes nothing: install_name_tool says
	// so as an error. What counts is read back by verifyLibs after.
	try := func(name string, args ...string) { _, _ = run(name, args...) }

	// Two halves on macOS. Each library says what it calls itself, and
	// that name is what anything linking it records, so the name has to
	// be made relative first. Then the program is told where to look.
	for _, lib := range libs {
		// Each library loses its local symbols, which nothing loads it
		// by and every update would download. Its exported symbols, the
		// ones a program links by, stay. The signature it breaks is
		// made again below. See docs/UPDATES.md.
		if _, err := run("strip", "-x", lib); err != nil {
			return err
		}
		try("install_name_tool", "-id", "@rpath/"+filepath.Base(lib), lib)
		// A library that calls another one records the same path.
		for _, other := range libs {
			try("install_name_tool", "-change", filepath.Join(from, filepath.Base(other)), "@rpath/"+filepath.Base(other), lib)
		}
	}
	for _, other := range libs {
		try("install_name_tool", "-change", filepath.Join(from, filepath.Base(other)), "@rpath/"+filepath.Base(other), program)
	}

	// Where @rpath means. Relative to the program, so it can be moved
	// anywhere. Three shapes: beside the program, in a folder beside it,
	// which is what bin/lib is, and Contents/Frameworks for a bundle.
	here, err := filepath.Abs(filepath.Dir(program))
	if err != nil {
		return err
	}
	there, err := filepath.Abs(libdir)
	if err != nil {
		return err
	}
	try("install_name_tool", "-add_rpath", "@executable_path", program)
	try("install_name_tool", "-add_rpath", "@executable_path/../Frameworks", program)
	if there != here {
		try("install_name_tool", "-add_rpath", "@executable_path/"+filepath.Base(there), program)
	}
	// And the one that kept the mistake invisible. cgo links the program
	// with an rpath of the module cache folder the libraries were built
	// into, so whatever else is wrong, the program finds them there and
	// runs perfectly on the machine that built it, the only machine where
	// that folder exists. The libraries call themselves @rpath/something,
	// and so does everything that links them, so every -change above
	// changes nothing on macOS. The whole question is the rpath, so the
	// module cache comes out of it.
	try("install_name_tool", "-delete_rpath", from, program)

	// Every rewrite above breaks the signature, so it is made again. Ad
	// hoc here, because the bundle is signed again as a whole.
	for _, f := range append([]string{program}, libs...) {
		try("codesign", "--remove-signature", f)
		try("codesign", "-s", "-", "-f", f)
	}
	return nil
}

// copyFile copies the file from to to, with mode.
func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// The mode of a file that was there before is not changed by
	// OpenFile.
	return os.Chmod(to, mode)
}

// verifyLibs reads a program back, rather than hoping, and says what
// would keep it from running on another Mac: a library it links as
// @rpath that is under none of the places it looks, or a place it looks
// that is the module cache of the Mac that built it, gomod.
//
// Every rewrite in carryLibs may fail quietly, and a run where they all
// failed looks exactly like a run where none of them was needed. And on
// macOS nothing records an absolute path: the libraries call themselves
// @rpath/something and the program records that, so the question that
// decides is whether each library it needs resolves under its rpaths.
// It used to be asked of otool -L, whose answer was always yes, while
// the module cache sat in LC_RPATH, which otool -L does not print.
func verifyLibs(program, gomod string) ([]string, error) {
	data, err := os.ReadFile(program)
	if err != nil {
		return nil, err
	}
	arches, err := archesOf(data)
	if err != nil {
		return nil, fmt.Errorf("%s %w", program, err)
	}
	if len(arches) == 0 {
		return nil, fmt.Errorf("%s is no Mac program", program)
	}
	dir, err := filepath.Abs(filepath.Dir(program))
	if err != nil {
		return nil, err
	}
	var found []string
	for _, a := range arches {
		var rpaths []string
		for _, l := range a.f.Loads {
			if r, ok := l.(*macho.Rpath); ok {
				rpaths = append(rpaths, r.Path)
			}
		}
		libs, err := a.f.ImportedLibraries()
		if err != nil {
			return nil, fmt.Errorf("the libraries of %s cannot be read: %w", program, err)
		}
		more, _ := otherLinks(a.f)
		for _, need := range append(libs, more...) {
			base, ok := strings.CutPrefix(need, "@rpath/")
			if !ok {
				continue
			}
			if !slices.ContainsFunc(rpaths, func(rp string) bool { return isFile(filepath.Join(rpathDir(rp, dir), base)) }) {
				found = append(found, fmt.Sprintf("needs %s and cannot find it", base))
			}
		}
		for _, rp := range rpaths {
			if gomod != "" && strings.Contains(rp, gomod) {
				found = append(found, "still looks in the build machine's module cache, which exists on no other machine: "+rp)
			}
		}
	}
	return tidy(found), nil
}

// rpathDir is the folder an rpath names for a program in dir.
func rpathDir(rp, dir string) string {
	if rest, ok := strings.CutPrefix(rp, "@executable_path"); ok {
		return dir + rest
	}
	if rest, ok := strings.CutPrefix(rp, "@loader_path"); ok {
		return dir + rest
	}
	return rp
}

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

// verified is verifyLibs as an error, or nil when nothing was found.
func verified(program, gomod string) error {
	found, err := verifyLibs(program, gomod)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("%s %s.\n  It would run here and nowhere else. See docs/BUILD.md", program, strings.Join(found, ", and "))
}

// goModCache is where the go command keeps modules.
func goModCache() (string, error) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMODCACHE: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// speechModuleDir is the speech module's folder and version, downloaded
// if it is not there, which go build has done before this runs.
func speechModuleDir() (dir, version string, err error) {
	out, err := exec.Command("go", "mod", "download", "-json", speechModule).Output()
	if err != nil {
		return "", "", fmt.Errorf("go mod download %s: %w", speechModule, err)
	}
	var m struct{ Dir, Version string }
	if err := json.Unmarshal(out, &m); err != nil {
		return "", "", err
	}
	if m.Dir == "" || m.Version == "" {
		return "", "", fmt.Errorf("go mod download says nothing of %s", speechModule)
	}
	return m.Dir, m.Version, nil
}

// carryLibsCommand is make's step after it builds the program on a Mac.
func carryLibsCommand(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: speakertrail-release carry-libs PROGRAM LIBDIR")
	}
	program, libdir := args[0], args[1]
	if runtime.GOOS != "darwin" {
		fmt.Fprintf(os.Stderr, "carry-libs: nothing to carry on %s\n", runtime.GOOS)
		return nil
	}
	if !isFile(program) {
		return fmt.Errorf("no such program: %s", program)
	}
	cache, err := goModCache()
	if err != nil {
		return err
	}
	module, version, err := speechModuleDir()
	if err != nil {
		return err
	}
	triple := "x86_64-apple-darwin"
	if runtime.GOARCH == "arm64" {
		triple = "aarch64-apple-darwin"
	}
	from := filepath.Join(module, "lib", triple)
	if _, err := os.Stat(from); err != nil {
		return fmt.Errorf("no speech library at %s", from)
	}
	// The script carried the module's libraries whenever fetching these
	// failed, and the bundle then refused them for espeak-ng. Now only a
	// system with no pin carries them, and a failed fetch stops here.
	ship := from
	if pin, ok := speechPins[runtime.GOOS+"/"+runtime.GOARCH]; ok {
		if ship, err = speechLibs(filepath.Join(".build", "speech"), version, pin, fetchHTTP, os.Stderr); err != nil {
			return err
		}
	} else {
		// Nothing ships from here yet. The module's own libraries do for
		// running on this machine, and the bundle refuses them.
		fmt.Fprintf(os.Stderr, "carry-libs: no speech library without espeak-ng is pinned for %s/%s, so this build carries the module's, which is not fit to ship. See docs/BUILD.md.\n", runtime.GOOS, runtime.GOARCH)
	}
	if err := carryLibs(program, libdir, from, ship, buildTools); err != nil {
		return err
	}
	return verified(program, cache)
}
