package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

var at = time.Date(2026, 11, 27, 21, 0, 0, 0, time.UTC)

const (
	headSHA = "1111111111111111111111111111111111111111"
	oldSHA  = "2222222222222222222222222222222222222222"
)

// A finished build is a build of main only when main's history holds it,
// and of a pull request only when the history of its newest commit does,
// whatever GitHub's record of the run says. Its version is the app's at
// that commit, with its channel and commit.
func TestWhatAFinishedBuildIsABuildOf(t *testing.T) {
	t.Parallel()
	gh := newFakeHub(at)
	gh.main = headSHA
	gh.histories[headSHA] = []string{oldSHA}
	gh.open = []openPull{{number: 58, title: "Small things\nand more", head: headSHA, branch: "small"}}
	gh.zips["7"] = 1
	gh.tree[oldSHA+":cmd/speakertrail-app/appfiles.go"] = []byte("package main\n\nconst appVersion = \"0.3.0\"\n")

	p, err := publishPlan(gh, "7", "push", "main", oldSHA)
	if err != nil {
		t.Fatal(err)
	}
	if !p.run || p.channel != "main" || p.version != "0.3.0-main.2222222" || p.commit != "222222222222" ||
		p.zip != "app-main-0.3.0-main.2222222.zip" || p.name != "main" {
		t.Errorf("main: %+v", p)
	}
	for _, event := range []string{"push", "pull_request"} {
		p, err = publishPlan(gh, "7", event, "small", oldSHA)
		if err != nil {
			t.Fatal(err)
		}
		if !p.run || p.channel != "pr-58" || p.version != "0.3.0-pr58.2222222" || p.name != "#58 Small things" {
			t.Errorf("%s of a pull request: %+v", event, p)
		}
	}
	// A tag pushed with the name main, at a commit main never held.
	if _, err := publishPlan(gh, "7", "push", "main", "3333333333333333333333333333333333333333"); err == nil {
		t.Error("a commit outside main's history was published as main's")
	}
	if p, err := publishPlan(gh, "7", "push", "gone", oldSHA); err != nil || p.run {
		t.Errorf("a branch with no pull request: %+v, %v", p, err)
	}
	if p, err := publishPlan(gh, "8", "push", "main", oldSHA); err != nil || p.run {
		t.Errorf("a run that left no build: %+v, %v", p, err)
	}
}

// packedBuild is a build as pack and sign leave it: a folder with its
// parts, its packed zip, two pieces and a patch, the zip and the entry.
//
// On the real disk: publish reads what pack and sign wrote as files.
func packedBuild(t *testing.T) (dir string, p updates.Parts) {
	t.Helper()
	dir = t.TempDir()
	p = updates.Parts{Parts: []updates.Part{
		{Name: "Contents/MacOS/speakertrail-app", SHA256: strings.Repeat("a", 64), Patches: []updates.Patch{{From: strings.Repeat("c", 64)}}},
		{Name: "Contents/Frameworks/libonnxruntime.dylib", SHA256: strings.Repeat("b", 64)},
	}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		partsFile: data, packedWhole: []byte("whole"), "app.zip": []byte("zip"), "entry.json": []byte("{}"),
		updates.PieceName(strings.Repeat("a", 64)):                          []byte("a"),
		updates.PieceName(strings.Repeat("b", 64)):                          []byte("b"),
		updates.PatchName(strings.Repeat("c", 64), strings.Repeat("a", 64)): []byte("patch"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, p
}

// The pieces the release lacks go up first, then the zip, its packed zip
// and its parts, then the entry, then the mark on the commit, and the
// patches last. A piece already there is not sent again. A patch that
// cannot go up costs no failure.
func TestABuildGoesUpBeforeItsEntryAndItsPatchesAfter(t *testing.T) {
	t.Parallel()
	dir, _ := packedBuild(t)
	gh := newFakeHub(at)
	gh.put(updates.PieceName(strings.Repeat("b", 64)), []byte("b"), at)
	patch := updates.PatchName(strings.Repeat("c", 64), strings.Repeat("a", 64))
	gh.refuse[patch] = true
	var log strings.Builder
	zip := "app-pr-58-0.3.0-pr58.2222222.zip"
	if err := publish(gh, filepath.Join(dir, "app.zip"), zip, dir, filepath.Join(dir, "entry.json"), "pr-58", oldSHA, &log); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"upload " + updates.PieceName(strings.Repeat("a", 64)),
		"upload " + zip,
		"upload " + updates.PackedName(zip),
		"upload " + updates.PartsName(zip),
		"upload channel-pr-58.json",
		"ready " + oldSHA + " success On the Updates page as pr-58",
	}
	if got := gh.doneWith(""); !slices.Equal(got, want) {
		t.Errorf("did\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(log.String(), "::warning::"+patch) {
		t.Errorf("the patch that did not go up was not said: %s", log.String())
	}
	// main's build marks no commit: its checks are main's own.
	gh = newFakeHub(at)
	if err := publish(gh, filepath.Join(dir, "app.zip"), "app-main.zip", dir, filepath.Join(dir, "entry.json"), "main", oldSHA, &log); err != nil {
		t.Fatal(err)
	}
	if got := gh.doneWith("ready"); len(got) != 0 {
		t.Errorf("main's commit was marked: %v", got)
	}
	if got := gh.doneWith("upload patch-"); len(got) != 1 {
		t.Errorf("the patch went up %v", got)
	}
}

// A build that was not published says why on its commit, the first thing
// a refusal names, or that a step failed.
func TestABuildNotPublishedSaysWhyOnItsCommit(t *testing.T) {
	t.Parallel()
	gh := newFakeHub(at)
	refusal := []byte("speakertrail-release: the build is refused:\n  Contents/MacOS/extra is a program it may not hold\n  and more\n")
	if err := notPublished(gh, oldSHA, refusal, "https://example.com/run/1"); err != nil {
		t.Fatal(err)
	}
	if err := notPublished(gh, headSHA, nil, "https://example.com/run/1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ready " + oldSHA + " failure Not signed: Contents/MacOS/extra is a program it may not hold",
		"ready " + headSHA + " failure Not published: a step of its publishing failed, see the run",
	}
	if got := gh.doneWith("ready"); !slices.Equal(got, want) {
		t.Errorf("marked %q", got)
	}
}
