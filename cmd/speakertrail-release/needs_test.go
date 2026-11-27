package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Whether a commit needs a build of its own, against the commit its
// channel's build was made from, in a repository of the test's own, so
// nothing is fetched and nothing is asked of GitHub. A mistake in it is
// silent: a commit gets no build and the Updates page goes on offering
// the one before, or it says a build is coming that never will. Every
// case is one a push really arrives as. They were scripts/needs-build-test.sh.
//
// On the real disk: git reads a repository.
func TestWhichCommitsNeedABuild(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
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
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) string {
		t.Helper()
		git("add", "-A")
		git("-c", "user.email=test@example.com", "-c", "user.name=test", "-c", "commit.gpgsign=false",
			"commit", "-q", "--allow-empty", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	git("init", "-q")
	write("internal/engine/run.go", "a")
	app := commit("app")
	// A file of the docs folder of the test's own repository, joined so
	// it reads as none of ours.
	write(filepath.Join("docs", "APP.md"), "b")
	docsOnly := commit("docs")
	write("README.md", "c")
	readme := commit("readme")
	write("internal/engine/run.go", "d")
	code := commit("code")
	empty := commit("nothing")
	// A change taken back: the files are the ones built, whatever
	// happened on the way.
	write("internal/engine/run.go", "a")
	back := commit("back")

	tr := gitTrees{dir: dir}
	for _, c := range []struct{ what, built, newest, want string }{
		{"a channel with no build", "", app, "build"},
		{"the commit that is built", app, app, "built"},
		{"the built commit, shortened", app[:12], app, "built"},
		{"a push of the docs", app, docsOnly, "docs"},
		{"a README is docs too", app, readme, "docs"},
		{"two pushes of docs, one after the other", docsOnly, readme, "docs"},
		{"code after docs", app, code, "build"},
		{"code, measured from what is built", docsOnly, code, "build"},
		{"nothing changed in the commit", code, empty, "built"},
		{"code changed and changed back", app, back, "docs"},
		{"a commit nobody can read", app, "0123456789abcdef0123456789abcdef01234567", "build"},
		{"a build nobody can read", "0123456789ab", code, "build"},
	} {
		if got, why := needsBuild(tr, c.built, c.newest); got != c.want {
			t.Errorf("%s: %s, want %s: %s", c.what, got, c.want, why)
		}
	}
}
