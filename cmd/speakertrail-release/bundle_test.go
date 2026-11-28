package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// aBin is bin/ as make leaves it before the bundle: the program pointed at
// its libraries, llama-server, its licence, the speech library, and the
// icon's source in the repository.
func aBin(t *testing.T, lib string) (root, bin string) {
	t.Helper()
	root = t.TempDir()
	bin = filepath.Join(root, "bin")
	exe := program{
		links:  []string{"/usr/lib/libSystem.B.dylib", "@rpath/libsherpa-onnx-c-api.dylib"},
		rpaths: []string{"@executable_path", "@executable_path/../Frameworks", "@executable_path/lib"},
	}
	for name, body := range map[string]string{
		filepath.Join(bin, "speakertrail-app"):                  string(exe.bytes()),
		filepath.Join(bin, "llama-server"):                      "llama",
		filepath.Join(bin, "LICENSE-llama.cpp"):                 "MIT",
		filepath.Join(bin, "lib", "libsherpa-onnx-c-api.dylib"): lib,
		filepath.Join(root, "build", "icon.png"):                "png",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, bin
}

// anIcon makes an .icns the way make-icon.sh would.
func anIcon(out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte("icns"), 0o644)
}

// The bundle holds the program, llama-server, its licence, the speech
// library and the icon where macOS and the app look for them, says what
// it is in its Info.plist, and is signed inside out and read back.
//
// On the real disk: the bundle is a folder, and codesign is recorded.
func TestTheAppIsBundled(t *testing.T) {
	t.Parallel()
	root, bin := aBin(t, "speech")
	r := &recorder{}
	app, err := bundleApp(root, bin, "0.1.0-pr135.4f97be5", bundler{run: r.run, icon: anIcon, gomod: "/go/pkg/mod"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(bin, "Speaker Trail.app"); app != want {
		t.Errorf("made %s, want %s", app, want)
	}
	c := filepath.Join(app, "Contents")
	for name, want := range map[string]string{
		"MacOS/llama-server":                    "llama",
		"Resources/LICENSE-llama.cpp":           "MIT",
		"Frameworks/libsherpa-onnx-c-api.dylib": "speech",
		"Resources/icon.icns":                   "icns",
		"PkgInfo":                               "APPL????",
	} {
		if b, err := os.ReadFile(filepath.Join(c, name)); err != nil || string(b) != want {
			t.Errorf("%s holds %q, %v, want %q", name, b, err, want)
		}
	}
	plist, err := os.ReadFile(filepath.Join(c, "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>0.1.0-pr135.4f97be5</string>", "<key>CFBundleIconFile</key>", "<string>speakertrail-app</string>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("the Info.plist has no %s", want)
		}
	}
	signed := []string{
		"codesign -s - -f " + filepath.Join(c, "MacOS", "llama-server"),
		"codesign -s - -f " + filepath.Join(c, "MacOS", "speakertrail-app"),
		"codesign --verify --strict " + app,
	}
	var got []string
	for _, l := range r.ran {
		if slices.Contains(signed, l) {
			got = append(got, l)
		}
	}
	if !slices.Equal(got, signed) {
		t.Errorf("signed %q, want %q in that order", got, signed)
	}
}

// A bundle without an icon to make has none in its Info.plist, and a
// version that is no plain text cannot break the Info.plist.
func TestABundleWithoutAnIcon(t *testing.T) {
	t.Parallel()
	root, bin := aBin(t, "speech")
	if err := os.Remove(filepath.Join(root, "build", "icon.png")); err != nil {
		t.Fatal(err)
	}
	icon := func(string) error { return errors.New("no icon is made without a source") }
	app, err := bundleApp(root, bin, "dev</string>", bundler{run: (&recorder{}).run, icon: icon})
	if err != nil {
		t.Fatal(err)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if strings.Contains(string(plist), "CFBundleIconFile") {
		t.Error("the Info.plist names an icon there is not")
	}
	if !strings.Contains(string(plist), "<string>dev&lt;/string&gt;</string>") {
		t.Errorf("the version is not escaped in %s", plist)
	}
	if _, err := plistKeys(plist); err != nil {
		t.Errorf("the Info.plist cannot be read: %v", err)
	}
}

// A bundle that would not work, or would ship what it must not, is not
// made, and says why.
func TestABundleThatWouldNotWorkIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		lib     string
		change  func(root, bin string) error
		failing []string
		want    string
	}{
		{"no program", "speech", func(_, bin string) error { return os.Remove(filepath.Join(bin, "speakertrail-app")) }, nil, "is not built"},
		{"no llama-server", "speech", func(_, bin string) error { return os.Remove(filepath.Join(bin, "llama-server")) }, nil, "no llama-server"},
		{"espeak-ng in the speech library", "speech espeak-ng", nil, nil, "carries espeak-ng"},
		{"no speech library", "speech", func(_, bin string) error { return os.RemoveAll(filepath.Join(bin, "lib")) }, nil, "needs libsherpa-onnx-c-api.dylib and cannot find it"},
		{"a signature that does not hold", "speech", nil, []string{"codesign --verify"}, "codesign --verify"},
		{"llama-server not signed", "speech", nil, []string{"codesign -s - -f " + "LLAMA"}, "llama-server"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root, bin := aBin(t, c.lib)
			if c.change != nil {
				if err := c.change(root, bin); err != nil {
					t.Fatal(err)
				}
			}
			var failing []string
			for _, f := range c.failing {
				failing = append(failing, strings.ReplaceAll(f, "LLAMA", filepath.Join(bin, "Speaker Trail.app", "Contents", "MacOS", "llama-server")))
			}
			_, err := bundleApp(root, bin, "dev", bundler{run: (&recorder{failing: failing}).run, icon: anIcon})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
}
