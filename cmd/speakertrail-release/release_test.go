package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// now is when the tests below run the clean-up.
var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

// sha is a SHA-256 made of one letter.
func sha(c string) string { return strings.Repeat(c, 64) }

// onDev writes the channel list and the release's files as the workflow
// has gh write them, and reads them back as the release tool does. The
// list points at the zip of each channel named.
//
// On the real disk: gh writes both, and the release tool reads them as files.
func onDev(t *testing.T, channels []string, assets []asset) dev {
	t.Helper()
	var list strings.Builder
	list.WriteString(`{"channels":[`)
	for i, c := range channels {
		if i > 0 {
			list.WriteString(",")
		}
		fmt.Fprintf(&list, `{"channel":%q,"url":"https://github.com/timmrkz/speakertrail/releases/download/dev/app-%s.zip"}`, c, c)
	}
	list.WriteString(`]}`)
	var files strings.Builder
	files.WriteString(`{"assets":[`)
	for i, a := range assets {
		if i > 0 {
			files.WriteString(",")
		}
		fmt.Fprintf(&files, `{"apiUrl":"https://api.github.com/x","contentType":"application/octet-stream","createdAt":%q,"name":%q,"size":1}`,
			a.CreatedAt.Format(time.RFC3339), a.Name)
	}
	files.WriteString(`]}`)
	dir := t.TempDir()
	listPath, assetsPath := filepath.Join(dir, "channels.json"), filepath.Join(dir, "assets.json")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetsPath, []byte(files.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := readRelease(listPath, assetsPath)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A new build is patched against every build the channel list names and
// the newest that left it, each once, and only those the release holds
// the parts of.
func TestTheBuildsPatchedAgainstAreTheListedAndTheNewestThatLeft(t *testing.T) {
	assets := []asset{
		{"app-main.zip", now.Add(-time.Hour)},
		{"app-main.parts.json", now.Add(-time.Hour)},
		{"app-pr-5.parts.json", now.Add(-48 * time.Hour)},
		{"channels.json", now},
	}
	var left []string
	for i := range leftBases + 3 {
		name := fmt.Sprintf("app-pr-%d.parts.json", 100+i)
		assets = append(assets, asset{name, now.Add(-time.Duration(i+2) * time.Hour)})
		left = append(left, name)
	}
	// pr-6 is listed and its parts are not there: a build from before parts.
	r := onDev(t, []string{"main", "pr-5", "pr-6"}, assets)

	got := r.bases()
	want := append([]string{"app-main.parts.json", "app-pr-5.parts.json"}, left[:leftBases]...)
	if !slices.Equal(got, want) {
		t.Errorf("the builds patched against are\n%v\nwant\n%v", got, want)
	}
}

// What no build an app may run reaches goes once it is an hour old: the
// zips the list no longer points at, the parts of the builds no longer
// patched against, and the pieces and patches no remaining parts name.
// The rest of the release stays.
func TestWhatNoBuildReachesGoesAfterAnHour(t *testing.T) {
	old := now.Add(-2 * time.Hour)
	assets := []asset{
		{"channels.json", old},
		{"channel-main.json", old},
		{"app-main.zip", old},
		{"app-main.zip.zst", old},
		{"app-main.parts.json", old},
		{"piece-" + sha("a") + ".zst", old},
		{"patch-" + sha("b") + "-" + sha("a") + ".bsdiff", old},
		// The build of a pull request just merged, which left the list.
		{"app-pr-7.zip", old},
		{"app-pr-7.zip.zst", old},
		{"app-pr-7.parts.json", now.Add(-90 * time.Minute)},
		{"piece-" + sha("b") + ".zst", old},
		// Named by nothing.
		{"piece-" + sha("c") + ".zst", old},
		{"patch-" + sha("c") + "-" + sha("a") + ".bsdiff", old},
		// Named by nothing, and only just uploaded by a publish beside this,
		// or exactly an hour ago.
		{"piece-" + sha("d") + ".zst", now.Add(-time.Minute)},
		{"app-pr-9.zip", now.Add(-time.Minute)},
		{"piece-" + sha("e") + ".zst", now.Add(-keptFor)},
		// Not the app's at all.
		{"pictures.zip", old},
		{"pr-notes.txt", old},
	}
	// Builds that left the list later than pr-7, all but one of the
	// newest, so the oldest of them is no longer patched against.
	for i := range leftBases {
		assets = append(assets, asset{fmt.Sprintf("app-pr-%d.parts.json", 20+i), now.Add(-time.Duration(80-i) * time.Minute)})
	}
	assets = append(assets, asset{"app-pr-1.parts.json", now.Add(-72 * time.Hour)})
	r := onDev(t, []string{"main"}, assets)
	parts := map[string]updates.Parts{
		"app-main.parts.json": {Parts: []updates.Part{
			{Bytes: []byte("PK")},
			{Name: "Speaker Trail.app/Contents/MacOS/speakertrail-app", SHA256: sha("a"), Patches: []updates.Patch{{From: sha("b")}}},
		}},
	}
	for i := range leftBases {
		parts[fmt.Sprintf("app-pr-%d.parts.json", 20+i)] = updates.Parts{}
	}
	if slices.Contains(r.bases(), "app-pr-7.parts.json") {
		t.Fatal("pr-7 is patched against, want it to have left the newest")
	}

	got := r.unreachable(parts, now)
	want := []string{
		"app-pr-1.parts.json",
		"app-pr-7.parts.json",
		"app-pr-7.zip",
		"app-pr-7.zip.zst",
		"patch-" + sha("c") + "-" + sha("a") + ".bsdiff",
		"piece-" + sha("b") + ".zst",
		"piece-" + sha("c") + ".zst",
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("what goes is\n%v\nwant\n%v", got, want)
	}
}

// When the parts of a build patched against cannot be read, nothing says
// which pieces and patches may go, so all of them stay. Zips nothing
// points at still go.
func TestPartsThatCannotBeReadKeepEveryPieceAndPatch(t *testing.T) {
	old := now.Add(-2 * time.Hour)
	r := onDev(t, []string{"main"}, []asset{
		{"app-main.zip", old},
		{"app-main.parts.json", old},
		{"app-pr-7.zip", old},
		{"piece-" + sha("c") + ".zst", old},
		{"patch-" + sha("c") + "-" + sha("a") + ".bsdiff", old},
	})

	got := r.unreachable(map[string]updates.Parts{}, now)
	if want := []string{"app-pr-7.zip"}; !slices.Equal(got, want) {
		t.Errorf("what goes is %v, want %v", got, want)
	}
}

// A list of the release's files that cannot be the whole of them, one
// without a zip the channel list points at or with a file of no time,
// names nothing, rather than files a build still needs.
func TestAListOfFilesThatCannotBeWholeRemovesNothing(t *testing.T) {
	old := now.Add(-2 * time.Hour)
	for name, assets := range map[string][]asset{
		"the listed zip missing": {{"app-pr-7.zip", old}},
		"a file of no time":      {{"app-main.zip", old}, {"app-pr-7.zip", old}, {"app-pr-8.zip", time.Time{}}},
	} {
		r := onDev(t, []string{"main"}, assets)
		r.assets = assets
		if got := r.unreachable(map[string]updates.Parts{}, now); got != nil {
			t.Errorf("with %s, %v goes, want nothing", name, got)
		}
	}
}

// A channel list or a list of files that cannot be read is an error, not
// a release with nothing on it, which would remove everything.
func TestAReleaseThatCannotBeReadIsAnError(t *testing.T) {
	dir := t.TempDir()
	notJSON := filepath.Join(dir, "not.json")
	if err := os.WriteFile(notJSON, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	fine := filepath.Join(dir, "fine.json")
	if err := os.WriteFile(fine, []byte(`{"channels":[],"assets":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{
		{filepath.Join(dir, "missing.json"), fine},
		{notJSON, fine},
		{fine, filepath.Join(dir, "missing.json")},
		{fine, notJSON},
	} {
		if _, err := readRelease(c[0], c[1]); err == nil {
			t.Errorf("readRelease(%s, %s) read a release", filepath.Base(c[0]), filepath.Base(c[1]))
		}
	}
}
