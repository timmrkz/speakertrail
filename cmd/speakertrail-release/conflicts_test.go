package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeGitHub stands in for git and GitHub: which commits conflict with
// main, what each pull request was told, and what is said now.
type fakeGitHub struct {
	open      []pull
	base      string
	conflicts map[string][]string // commit to its files in conflict
	broken    map[string]bool     // commits git cannot try
	said      map[int]string
	told      map[int][]string
}

func (f *fakeGitHub) pulls() ([]pull, error)    { return f.open, nil }
func (f *fakeGitHub) main() (string, error)     { return f.base, nil }
func (f *fakeGitHub) fetch(commit string) error { return nil }
func (f *fakeGitHub) merge(commit string) ([]string, error) {
	if f.broken[commit] {
		return nil, errors.New("exit 128: fatal: bad object")
	}
	return f.conflicts[commit], nil
}
func (f *fakeGitHub) comments(n int) (string, error) { return f.said[n], nil }
func (f *fakeGitHub) comment(n int, body string) error {
	if f.told == nil {
		f.told = map[int][]string{}
	}
	f.told[n] = append(f.told[n], body)
	if f.said == nil {
		f.said = map[int]string{}
	}
	f.said[n] += body + "\n"
	return nil
}

const (
	mainA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mainB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pr1   = "1111111111111111111111111111111111111111"
	pr2   = "2222222222222222222222222222222222222222"
	pr3   = "3333333333333333333333333333333333333333"
	fork  = "ffffffffffffffffffffffffffffffffffffffff"
)

// After a push to main, every pull request of this repository that no
// longer merges is told, with the files that conflict, and the one that
// merges and the one from a fork are not.
func TestAPushToMainTellsEachPullRequestThatConflicts(t *testing.T) {
	t.Parallel()
	g := &fakeGitHub{
		base: mainA,
		open: []pull{
			{Number: 44, Branch: "drafts", Head: pr1},
			{Number: 46, Branch: "flat", Head: pr2},
			{Number: 50, Branch: "theirs", Head: fork, Fork: true},
		},
		conflicts: map[string][]string{pr1: {"Makefile", "internal/store/store.go"}, fork: {"Makefile"}},
	}
	var log strings.Builder
	if err := tellConflicts(g, "", "", &log); err != nil {
		t.Fatal(err)
	}
	if len(g.told[46]) != 0 || len(g.told[50]) != 0 {
		t.Errorf("told a pull request that merges or one from a fork: %v", g.told)
	}
	if len(g.told[44]) != 1 {
		t.Fatalf("#44 was told %d times, want once", len(g.told[44]))
	}
	want := "This pull request at 1111111 no longer merges into main at aaaaaaa. These files conflict:\n\n" +
		"- `Makefile`\n- `internal/store/store.go`\n\n<!-- conflicts " + pr1 + " " + mainA + " -->"
	if g.told[44][0] != want {
		t.Errorf("#44 was told\n%s\nwant\n%s", g.told[44][0], want)
	}
	if !strings.Contains(log.String(), "#46 at 2222222 merges") {
		t.Errorf("the log does not say #46 merges:\n%s", log.String())
	}
}

// One comment lost on its way to the session is not the last it hears:
// the same two commits tell a pull request once, a new commit of main or
// of the pull request tells it again.
func TestAConflictIsToldAgainWhenMainMoves(t *testing.T) {
	t.Parallel()
	g := &fakeGitHub{
		base:      mainA,
		open:      []pull{{Number: 44, Branch: "drafts", Head: pr1}},
		conflicts: map[string][]string{pr1: {"Makefile"}, pr3: {"Makefile"}},
	}
	tell := func(branch, pushed string) {
		t.Helper()
		if err := tellConflicts(g, branch, pushed, &strings.Builder{}); err != nil {
			t.Fatal(err)
		}
	}
	tell("", "")
	tell("", "")
	if len(g.told[44]) != 1 {
		t.Fatalf("the same two commits told #44 %d times, want once", len(g.told[44]))
	}
	g.base = mainB
	tell("", "")
	if len(g.told[44]) != 2 {
		t.Fatalf("a new commit of main told #44 %d times in all, want twice", len(g.told[44]))
	}
	if !strings.Contains(g.told[44][1], "<!-- conflicts "+pr1+" "+mainB+" -->") {
		t.Errorf("the second comment does not mark the new main: %s", g.told[44][1])
	}
	tell("drafts", pr3)
	if len(g.told[44]) != 3 {
		t.Fatalf("a new commit of #44 told it %d times in all, want three", len(g.told[44]))
	}
}

// After a push to a branch only that branch's pull request is tried, at
// the commit pushed, which GitHub may not show as its head yet.
func TestAPushToABranchTriesItsPullRequestAtTheCommitPushed(t *testing.T) {
	t.Parallel()
	g := &fakeGitHub{
		base: mainA,
		open: []pull{
			{Number: 44, Branch: "drafts", Head: pr1},
			{Number: 46, Branch: "flat", Head: pr2},
		},
		conflicts: map[string][]string{pr2: {"go.mod"}, pr3: {"Makefile"}},
	}
	if err := tellConflicts(g, "drafts", pr3, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if len(g.told[46]) != 0 {
		t.Errorf("a push to drafts told #46, whose branch was not pushed")
	}
	if len(g.told[44]) != 1 || !strings.Contains(g.told[44][0], "at 3333333 ") {
		t.Errorf("#44 was not told at the commit pushed: %q", g.told[44])
	}
}

// A pull request git cannot try is a warning, and the rest are still
// tried.
func TestAPullRequestGitCannotTryLeavesTheOthers(t *testing.T) {
	t.Parallel()
	g := &fakeGitHub{
		base: mainA,
		open: []pull{
			{Number: 44, Branch: "drafts", Head: pr1},
			{Number: 46, Branch: "flat", Head: pr2},
		},
		broken:    map[string]bool{pr1: true},
		conflicts: map[string][]string{pr2: {"Makefile"}},
	}
	var log strings.Builder
	if err := tellConflicts(g, "", "", &log); err != nil {
		t.Fatal(err)
	}
	if len(g.told[44]) != 0 {
		t.Errorf("#44, which git could not try, was told it conflicts")
	}
	if !strings.Contains(log.String(), "::warning::#44: git merge-tree could not merge it") {
		t.Errorf("no warning for #44:\n%s", log.String())
	}
	if len(g.told[46]) != 1 {
		t.Errorf("#46 was told %d times after #44 failed, want once", len(g.told[46]))
	}
}

func TestMergeTreeFiles(t *testing.T) {
	t.Parallel()
	tree := "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	for _, c := range []struct {
		name  string
		out   string
		code  int
		files []string
		fails bool
	}{
		{"merges", tree + "\n", 0, nil, false},
		{"conflicts", tree + "\nMakefile\ninternal/store/store.go\n", 1, []string{"Makefile", "internal/store/store.go"}, false},
		{"sha-256", strings.Repeat("c", 64) + "\nMakefile\n", 1, []string{"Makefile"}, false},
		{"no tree", "fatal: refusing to merge unrelated histories\n", 1, nil, true},
		{"no file", tree + "\n", 1, nil, true},
		{"git failed", "fatal: bad object\n", 128, nil, true},
	} {
		files, err := mergeTreeFiles(c.out, c.code)
		if (err != nil) != c.fails || !slices.Equal(files, c.files) {
			t.Errorf("%s: got %q, %v, want %q, fails %v", c.name, files, err, c.files, c.fails)
		}
	}
}

// The real git merges a branch that merges and names the file of one that
// does not, the way the workflow runs it, in a checkout of main.
//
// On the real disk: git merge-tree is the program under test, and it
// reads a repository on the disk.
func TestGitMergeTreeNamesTheFilesInConflict(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("Makefile", "walks:\n\tnode walks.mjs\n")
	write("README.md", "Speaker Trail\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	git("checkout", "-q", "-b", "clean")
	write("README.md", "Speaker Trail, on a Mac\n")
	git("commit", "-q", "-am", "readme")
	clean := git("rev-parse", "HEAD")
	git("checkout", "-q", "-b", "conflicting", "main")
	write("Makefile", "walks:\n\tnode walks.mjs drafts.mjs\n")
	git("commit", "-q", "-am", "drafts")
	conflicting := git("rev-parse", "HEAD")
	git("checkout", "-q", "main")
	write("Makefile", "walks:\n\tnode walks.mjs updates.mjs\n")
	git("commit", "-q", "-am", "updates")

	g := tools{dir: dir}
	if files, err := g.merge(clean); err != nil || len(files) != 0 {
		t.Errorf("a branch that merges: %q, %v", files, err)
	}
	if files, err := g.merge(conflicting); err != nil || !slices.Equal(files, []string{"Makefile"}) {
		t.Errorf("a branch that conflicts: %q, %v, want Makefile", files, err)
	}
	if _, err := g.merge(strings.Repeat("0", 40)); err == nil {
		t.Error("a commit that is not there merged")
	}
}
