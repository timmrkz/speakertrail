package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// devid is a program signed the way scripts/sign-macos.sh signs it: with
// a Developer ID of the team given, and the hardened runtime.
func devid(p program, team string) program {
	p.signed, p.flags, p.team = true, flagRuntime, team
	return p
}

// asMade is a program signed ad hoc, the way make signs it.
func asMade(p program) program {
	p.signed, p.flags = true, flagAdhoc
	return p
}

// speech is the speech library the app carries, as it links today.
var speech = program{links: []string{"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib"}}

const (
	exe = "Speaker Trail.app/Contents/MacOS/speakertrail-app"
	lib = "Speaker Trail.app/Contents/Frameworks/libsherpa-onnx-c-api.dylib"
)

// Once publish.yml has the Developer ID, a build is signed for updates
// only when every program in it is signed with the hardened runtime by
// one team. A build that lost it on the way is refused, and no entry is
// written for it, so no app is offered it. Without -hardened, a build
// signed ad hoc is signed as before.
//
// On the real disk: the release tool reads the zip and writes the entry as files.
func TestABuildWithoutTheHardenedRuntimeIsNotSigned(t *testing.T) {
	adhocRuntime := devid(app, "")
	adhocRuntime.flags |= flagAdhoc
	cases := []struct {
		name     string
		exe, lib program
		hardened bool
		want     string
	}{
		{"one team, the hardened runtime", devid(app, "TEAM123456"), devid(speech, "TEAM123456"), true, ""},
		{"signed ad hoc, as made, without -hardened", asMade(adhoc), asMade(speech), false, ""},
		{"signed ad hoc, as made", asMade(adhoc), asMade(speech), true, exe + ": is signed ad hoc"},
		{"ad hoc with the hardened runtime", adhocRuntime, devid(speech, "TEAM123456"), true, exe + ": is signed ad hoc"},
		{"the app without the hardened runtime",
			func() program { p := devid(app, "TEAM123456"); p.flags = 0; return p }(),
			devid(speech, "TEAM123456"), true, exe + ": is signed without the hardened runtime"},
		{"a library left unsigned", devid(app, "TEAM123456"), speech, true, lib + ": is not signed"},
		{"a library of another team", devid(app, "TEAM123456"), devid(speech, "OTHER98765"), true, lib + ": is signed by team OTHER98765"},
		{"signed by no team", devid(app, ""), devid(speech, ""), true, exe + ": is signed by no team"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release(t)
			files := theApp("main")
			files[exe] = c.exe.bytes()
			files[lib] = c.lib.bytes()
			zipPath := filepath.Join(t.TempDir(), "app.zip")
			if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "main.json")
			args := []string{"-zip", zipPath, "-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234",
				"-commit", "abc1234def56", "-url", "https://example.com/main.zip", "-out", out}
			if c.hardened {
				args = append(args, "-hardened")
			}
			err := sign(args)
			_, written := os.Stat(out)
			if c.want == "" {
				if err != nil || written != nil {
					t.Errorf("signing it said %v, and the entry %v", err, written)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("signing it said %v, want %q", err, c.want)
			}
			if !os.IsNotExist(written) {
				t.Error("an entry was written for it")
			}
		})
	}
}
