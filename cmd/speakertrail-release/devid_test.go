package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The signing with the Developer ID runs the Mac's own tools, which no
// test here has. These hand in tools of their own: security answers and
// does nothing, and codesign writes into each file it signs a program
// signed the way the test says, so what harden reads back is what such a
// codesign would have left.

// fakeMac records every command and signs the way it is told.
type fakeMac struct {
	commands [][]string
	// flags and microphone are how codesign signs.
	flags      uint32
	microphone bool
	// failOn makes the command whose last argument ends with it fail.
	failOn string
}

func (m *fakeMac) tools() tools {
	return tools{
		run: func(name string, args ...string) ([]byte, error) {
			m.commands = append(m.commands, append([]string{name}, args...))
			if m.failOn != "" && strings.HasSuffix(args[len(args)-1], m.failOn) {
				return nil, errors.New(name + " failed")
			}
			if name == "security" && args[0] == "list-keychains" && len(args) == 3 {
				return []byte("    \"/Users/runner/Library/Keychains/login.keychain-db\"\n    \"/Library/Keychains/System.keychain\"\n"), nil
			}
			if name == "codesign" && slices.Contains(args, "-s") {
				target := args[len(args)-1]
				p := speech
				if strings.HasSuffix(target, ".app") {
					p = app
					if m.microphone {
						p.entitled = plistHead + "<key>com.apple.security.device.audio-input</key><true/>\n</dict>\n</plist>\n"
					}
					target = filepath.Join(target, "Contents", "MacOS", appProgram)
				}
				p.signed, p.flags, p.team = true, m.flags, "TEAM123456"
				return nil, os.WriteFile(target, p.bytes(), 0o755)
			}
			return nil, nil
		},
		fetch: func(url string) ([]byte, error) { return []byte("a certificate"), nil },
	}
}

// signedBy are the targets of codesign's signing commands, in order.
func (m *fakeMac) signed() [][]string {
	var out [][]string
	for _, c := range m.commands {
		if c[0] == "codesign" && slices.Contains(c, "-s") {
			out = append(out, c)
		}
	}
	return out
}

// bundle lays out the app as make builds it: the program, llama-server
// and the speech libraries, signed ad hoc.
func bundle(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), appName)
	files := map[string]program{
		"Contents/MacOS/" + appProgram:                     asMade(adhoc),
		"Contents/MacOS/llama-server":                      asMade(speech),
		"Contents/Frameworks/libonnxruntime.dylib":         asMade(speech),
		"Contents/Frameworks/libsherpa-onnx-c-api.dylib":   asMade(speech),
		"Contents/Frameworks/libsherpa-onnx-cxx-api.dylib": asMade(speech),
	}
	for name, p := range files {
		file := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, p.bytes(), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

func signer(m *fakeMac) devID {
	return devID{tools: m.tools(), p12: "developer-id.p12", password: "the password",
		identity: "Developer ID Application", entitlements: "build/entitlements.plist", timestamp: true}
}

// Every library and llama-server are signed before the app, each with
// the hardened runtime, Apple's time stamp and the Developer ID from the
// keychain made for it, and only the app with the entitlements. The
// keychain is gone at the end, and the Mac's own list of keychains is as
// it was.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestTheAppIsSignedInsideOutInAKeychainOfItsOwn(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true}
	app := bundle(t)
	if err := signer(m).signApp(app); err != nil {
		t.Fatal(err)
	}
	var targets []string
	signed := m.signed()
	for i, c := range signed {
		targets = append(targets, strings.TrimPrefix(c[len(c)-1], app))
		for _, want := range []string{"--options", "runtime", "--timestamp", "--keychain", "Developer ID Application"} {
			if !slices.Contains(c, want) {
				t.Errorf("%s was signed without %s: %q", c[len(c)-1], want, c)
			}
		}
		if slices.Contains(c, "--entitlements") != (i == len(signed)-1) {
			t.Errorf("%s: entitlements %v", c[len(c)-1], slices.Contains(c, "--entitlements"))
		}
	}
	want := []string{
		"/Contents/Frameworks/libonnxruntime.dylib",
		"/Contents/Frameworks/libsherpa-onnx-c-api.dylib",
		"/Contents/Frameworks/libsherpa-onnx-cxx-api.dylib",
		"/Contents/MacOS/llama-server",
		"",
	}
	if !slices.Equal(targets, want) {
		t.Errorf("signed %q, want %q", targets, want)
	}
	last := m.commands[len(m.commands)-2:]
	if !slices.Equal(last[0], []string{"security", "list-keychains", "-d", "user", "-s",
		"/Users/runner/Library/Keychains/login.keychain-db", "/Library/Keychains/System.keychain"}) {
		t.Errorf("the list of keychains was left as %q", last[0])
	}
	if last[1][1] != "delete-keychain" {
		t.Errorf("the last command was %q, not the keychain removed", last[1])
	}
}

// A signing that fails half way leaves no keychain behind with the
// certificate in it, and the Mac's list as it was.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestAFailedSigningLeavesNoKeychainBehind(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true, failOn: "llama-server"}
	err := signer(m).signApp(bundle(t))
	if err == nil {
		t.Fatal("a signing that failed said nothing")
	}
	last := m.commands[len(m.commands)-2:]
	if last[0][1] != "list-keychains" || last[1][1] != "delete-keychain" {
		t.Errorf("after the failure it ran %q", last)
	}
}

// What codesign left is read back as the release check reads it, so an
// app it signed without the hardened runtime, ad hoc or without the
// microphone is refused here, before it is packed.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestAnAppSignedWrongIsRefused(t *testing.T) {
	cases := []struct {
		name       string
		flags      uint32
		microphone bool
		want       string
	}{
		{"without the hardened runtime", 0, true, "llama-server is not signed with the hardened runtime"},
		{"ad hoc", flagRuntime | flagAdhoc, true, "libonnxruntime.dylib is not signed with the hardened runtime"},
		{"without the microphone", flagRuntime, false, "signed without the microphone"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := signer(&fakeMac{flags: c.flags, microphone: c.microphone}).signApp(bundle(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("signing it said %v, want %q", err, c.want)
			}
		})
	}
}

// Until build/team-id.txt names Tim's team, a build stays signed ad hoc
// and the run says so. Once it does, a build without the certificate
// fails, never quietly.
//
// On the real disk: the team's file and the workflow's output are files.
func TestHardenGoesByTheTeamFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "output")
	t.Setenv("GITHUB_OUTPUT", out)
	t.Setenv("SPEAKERTRAIL_DEVELOPER_ID", "")
	if err := harden([]string{"-zip", filepath.Join(dir, "app.zip"), "-team-file", filepath.Join(dir, "none.txt")}); err != nil {
		t.Errorf("with no team it said %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("with no team it named one to the next steps")
	}
	team := filepath.Join(dir, "team-id.txt")
	if err := os.WriteFile(team, []byte("TEAM123456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := harden([]string{"-zip", filepath.Join(dir, "app.zip"), "-team-file", team})
	if err == nil || !strings.Contains(err.Error(), "SPEAKERTRAIL_DEVELOPER_ID is not set") {
		t.Errorf("with a team and no certificate it said %v", err)
	}
}
