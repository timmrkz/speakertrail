package main

import (
	"errors"
	"strings"
	"testing"
)

// fakeMachine is a computer with the programs named in has, each of which
// answers output with what answers holds for its arguments.
func fakeMachine(goos, goVersion string, has []string, answers map[string]string) machine {
	return machine{
		goos:      goos,
		goVersion: goVersion,
		has: func(name string) bool {
			for _, h := range has {
				if h == name {
					return true
				}
			}
			return false
		},
		output: func(name string, args ...string) (string, error) {
			answer, ok := answers[name+" "+strings.Join(args, " ")]
			if !ok {
				return "", errors.New("exit status 1")
			}
			return answer, nil
		},
	}
}

var (
	nodeMajor = `node -p process.versions.node.split(".")[0]`
	nodeV     = "node -v"
)

// A build stops before it starts, with the command that fixes it, on a
// machine that misses any one thing a build needs, and goes ahead on one
// that has them all.
func TestABuildNeedsEverythingItNeeds(t *testing.T) {
	t.Parallel()
	mac := map[string]string{nodeMajor: "22", nodeV: "v22.12.0", "xcode-select -p": "/Library/Developer/CommandLineTools"}
	without := func(m map[string]string, key string) map[string]string {
		c := map[string]string{}
		for k, v := range m {
			if k != key {
				c[k] = v
			}
		}
		return c
	}
	with := func(m map[string]string, key, value string) map[string]string {
		c := without(m, key)
		c[key] = value
		return c
	}
	for _, c := range []struct {
		name string
		m    machine
		// fix is the command said for what is missing, or empty when
		// nothing is.
		fix string
	}{
		{"a Mac with everything", fakeMachine("darwin", "go1.27.1", []string{"node", "cmake"}, mac), ""},
		{"a newer Go", fakeMachine("darwin", "go1.28rc1", []string{"node", "cmake"}, mac), ""},
		{"an old Go", fakeMachine("darwin", "go1.26.3", []string{"node", "cmake"}, mac), "brew upgrade go"},
		{"no Node.js", fakeMachine("darwin", "go1.27.1", []string{"cmake"}, mac), "brew install node"},
		{"an old Node.js", fakeMachine("darwin", "go1.27.1", []string{"node", "cmake"}, with(with(mac, nodeMajor, "20"), nodeV, "v20.1.0")), "brew upgrade node"},
		{"a Node.js that says nothing", fakeMachine("darwin", "go1.27.1", []string{"node", "cmake"}, without(mac, nodeMajor)), "brew upgrade node"},
		{"no command line tools", fakeMachine("darwin", "go1.27.1", []string{"node", "cmake"}, without(mac, "xcode-select -p")), "xcode-select --install"},
		{"no CMake", fakeMachine("darwin", "go1.27.1", []string{"node"}, mac), "brew install cmake"},
		{"Linux with GTK", fakeMachine("linux", "go1.27.1", []string{"node"}, with(mac, "pkg-config --exists gtk4 webkitgtk-6.0", "")), ""},
		{"Linux without GTK", fakeMachine("linux", "go1.27.1", []string{"node"}, mac), "sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev"},
	} {
		var out strings.Builder
		err := report(&out, survey(c.m), false)
		if c.fix == "" {
			if err != nil || out.Len() != 0 {
				t.Errorf("%s: want silence before a build, got %v and %q", c.name, err, out.String())
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: the build went ahead, said %q", c.name, out.String())
		}
		if !strings.Contains(out.String(), "missing") || !strings.Contains(out.String(), c.fix) {
			t.Errorf("%s: want %q said for what is missing, got %q", c.name, c.fix, out.String())
		}
		if strings.Contains(out.String(), "  ok ") {
			t.Errorf("%s: before a build only what is missing is said, got %q", c.name, out.String())
		}
	}
}

// make check says everything it found, and never fails, so it can be run
// to see what is there.
func TestCheckSaysEverythingAndNeverFails(t *testing.T) {
	t.Parallel()
	all := fakeMachine("darwin", "go1.27.1", []string{"node", "cmake"},
		map[string]string{nodeMajor: "22", nodeV: "v22.12.0", "xcode-select -p": "/x"})
	var out strings.Builder
	if err := report(&out, survey(all), true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ok       Go 1.27.1", "ok       Node.js v22.12.0", "ok       the command line tools", "ok       CMake", "Everything is here."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("want %q in %q", want, out.String())
		}
	}

	none := fakeMachine("darwin", "go1.26.0", nil, nil)
	out.Reset()
	if err := report(&out, survey(none), true); err != nil {
		t.Errorf("make check failed: %v", err)
	}
	if strings.Contains(out.String(), "Everything is here.") || strings.Count(out.String(), "missing") != 4 {
		t.Errorf("want four things missing, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Go 1.27 or newer, found 1.26.0") {
		t.Errorf("want the Go found said, got %q", out.String())
	}
}
