package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A signed build fits only the checkout whose key it carries. One that
// carries none, or whose key cannot be worked out, never fits, so the job
// builds the tool itself.
func TestOnlyABuildOfThisCodeFits(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, stamp, key string
		err              error
		want             bool
	}{
		{"the same code", "0123456789abcdef", "0123456789abcdef", nil, true},
		{"other code", "0123456789abcdef", "fedcba9876543210", nil, false},
		{"a build that says nothing", "", "", nil, false},
		{"a checkout that cannot be read", "0123456789abcdef", "", errors.New("no git"), false},
	} {
		if got, why := fits(c.stamp, c.key, c.err); got != c.want {
			t.Errorf("%s: fits %v: %s", c.name, got, why)
		}
	}
}

// The tool is put on the job's path only when it fits, and the job is
// told either way.
//
// On the real disk: GitHub reads the outputs and the path from files.
func TestOnlyAToolThatFitsGoesOnThePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		stamp, key string
		want       string
	}{
		{"0123456789abcdef", "0123456789abcdef", "fits=true\n"},
		{"0123456789abcdef", "fedcba9876543210", "fits=false\n"},
	} {
		outputs, path := filepath.Join(dir, c.want[:6]+"out"), filepath.Join(dir, c.want[:6]+"path")
		var said strings.Builder
		if err := answerFits(&said, c.stamp, c.key, nil, "/tool", outputs, path); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(outputs); string(got) != c.want {
			t.Errorf("said %q, want %q", got, c.want)
		}
		got, err := os.ReadFile(path)
		if c.want == "fits=true\n" && string(got) != "/tool\n" {
			t.Errorf("the path holds %q", got)
		}
		if c.want == "fits=false\n" && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a tool that does not fit went on the path: %q", got)
		}
	}
}

// The tool is built from go.mod, go.sum and the folders of this module it
// imports, on Linux and on macOS, as Go says, and its key changes with a
// file in any of them and with nothing else.
//
// On the real disk: go list and git read the module and a repository.
func TestTheToolsKeyIsWhatItIsBuiltFrom(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	inputs, err := toolInputs("../..", "./cmd/speakertrail-release")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go.mod", "go.sum", "cmd/speakertrail-release", "internal/updates"} {
		if !slices.Contains(inputs, want) {
			t.Errorf("the inputs %v leave out %s", inputs, want)
		}
	}
	if slices.Contains(inputs, "cmd/speakertrail-app") || slices.ContainsFunc(inputs, func(s string) bool { return strings.HasPrefix(s, "..") }) {
		t.Errorf("the inputs %v hold what the tool is not built from", inputs)
	}

	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(name, text string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "-A")
		git("-c", "user.email=test@example.com", "-c", "user.name=test", "-c", "commit.gpgsign=false", "commit", "-q", "-m", name)
		return git("rev-parse", "HEAD")
	}
	git("init", "-q")
	commit("go.sum", "")
	commit("cmd/tool/main.go", "package main")
	first := commit("go.mod", "module x")
	elsewhere := commit("cmd/app/main.go", "package main")
	changed := commit("cmd/tool/main.go", "package main\n\nimport _ \"x/lib\"")
	in := []string{"go.mod", "go.sum", "cmd/tool"}
	key := func(commit string) string {
		t.Helper()
		k, err := gitKey(dir, commit, in)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	if key(first) != key(elsewhere) {
		t.Error("a change outside what the tool is built from changed its key")
	}
	if key(first) == key(changed) {
		t.Error("a change to the tool left its key as it was")
	}
	if _, err := gitKey(dir, changed, append(in, "lib")); err == nil {
		t.Error("a folder that is not there gave a key")
	}
}

// main's new builds go into the release release-tool, and everything
// else there goes.
func TestKeepingTheToolLeavesOnlyMainsNewBuilds(t *testing.T) {
	t.Parallel()
	gh := newFakeHub(time.Date(2026, 11, 27, 21, 0, 0, 0, time.UTC))
	gh.put("speakertrail-release-Linux-X64", []byte("old"), gh.now)
	gh.put("speakertrail-release-0123", []byte("from before"), gh.now)
	var said strings.Builder
	builds := map[string][]byte{"speakertrail-release-Linux-X64": []byte("linux"), "speakertrail-release-macOS-ARM64": []byte("mac")}
	if err := keepTool(gh, builds, &said); err != nil {
		t.Fatal(err)
	}
	if len(gh.files) != 2 || string(gh.files["speakertrail-release-Linux-X64"]) != "linux" || string(gh.files["speakertrail-release-macOS-ARM64"]) != "mac" {
		t.Errorf("the release holds %v", gh.files)
	}
	if err := keepTool(gh, map[string][]byte{"channels.json": nil}, &said); err == nil {
		t.Error("kept a file that is no build of the tool")
	}
}
