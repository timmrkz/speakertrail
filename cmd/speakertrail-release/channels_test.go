package main

import (
	"crypto/ed25519"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// Writing the list takes away the entry of a pull request that is not
// open any more, says which open one has a newer commit on its way and
// which no longer merges into main, and leaves out a commit whose build
// already ended without one. It replaces only the older of the list's
// two copies, and removes what no build reaches.
//
// On the real disk: the entries are signed as files, see signed.
func TestWritingTheListSaysWhatIsComingAndTakesAwayWhatEnded(t *testing.T) {
	key := release(t)
	gh := newFakeHub(at)
	for _, channel := range []string{"main", "pr-58", "pr-59", "pr-60"} {
		data, err := os.ReadFile(signed(t, channel, channel))
		if err != nil {
			t.Fatal(err)
		}
		gh.put("channel-"+channel+".json", data, at.Add(-time.Hour))
	}
	gh.put("channels-a.json", []byte("{}"), at.Add(-2*time.Hour))
	gh.put("channels-b.json", []byte("{}"), at.Add(-time.Hour))
	gh.put("app-pr-12-0.1.0-pr12.0000000.zip", []byte("zip"), at.Add(-2*time.Hour))
	gh.main = "abc1234def56"
	built := "abc1234def56"
	gh.open = []openPull{
		{number: 58, head: headSHA, branch: "a"},
		{number: 60, head: oldSHA, branch: "c"},
	}
	gh.conflict[58] = true
	// pr-60's newest commit failed to build, so nothing is coming.
	gh.runs[oldSHA] = [2]string{"completed", "failure"}
	gh.gone = []endedPull{{59, true}}
	tr := fakeTrees{{built, headSHA}: {"cmd/speakertrail-app/main.go"}, {built, oldSHA}: {"internal/x.go"}}
	var log strings.Builder
	none := func([]string) map[string]updates.Parts { return map[string]updates.Parts{} }
	if err := writeChannels(gh, tr, key.Public().(ed25519.PublicKey), none, at, &log); err != nil {
		t.Fatal(err)
	}
	if _, there := gh.files["channel-pr-59.json"]; there {
		t.Error("the entry of a pull request that was merged stayed")
	}
	l, err := updates.Parse(gh.files["channels-a.json"])
	if err != nil {
		t.Fatalf("the older copy is not a list: %v", err)
	}
	if string(gh.files["channels-b.json"]) != "{}" {
		t.Error("the newer copy was replaced")
	}
	var channels []string
	for _, b := range l.Channels {
		channels = append(channels, b.Channel)
	}
	slices.Sort(channels)
	if strings.Join(channels, " ") != "main pr-58 pr-60" {
		t.Errorf("the list holds %v", channels)
	}
	if b, _ := l.Find("pr-58"); b.Newest != headSHA[:12] || !b.Conflict {
		t.Errorf("pr-58: newest %q, conflict %v", b.Newest, b.Conflict)
	}
	if b, _ := l.Find("pr-60"); b.Newest != "" {
		t.Errorf("pr-60's newest is %q, though its build failed", b.Newest)
	}
	if b, _ := l.Find("main"); b.Newest != "" {
		t.Errorf("main is built and has %q coming", b.Newest)
	}
	if l.Ended("pr-59") != "merged" {
		t.Errorf("pr-59 is said to be %q", l.Ended("pr-59"))
	}
	if !slices.Contains(gh.doneWith("rename"), "rename channels.next.json channels.json") {
		t.Errorf("channels.json was not renamed into place: %v", gh.doneWith(""))
	}
	if !slices.Contains(gh.doneWith("remove"), "remove app-pr-12-0.1.0-pr12.0000000.zip") {
		t.Errorf("a zip no entry points at stayed: %v", gh.doneWith("remove"))
	}
}

// A publish that has waited for the environment for half an hour is
// cancelled, and one that waited less is not.
func TestAPublishStuckBeforeTheEnvironmentIsCancelled(t *testing.T) {
	t.Parallel()
	gh := newFakeHub(at)
	gh.waits = []waitingRun{{1, at.Add(-31 * time.Minute)}, {2, at.Add(-5 * time.Minute)}}
	var log strings.Builder
	if err := unstick(gh, at, &log); err != nil {
		t.Fatal(err)
	}
	if got := gh.doneWith("cancel"); !slices.Equal(got, []string{"cancel 1"}) {
		t.Errorf("cancelled %v", got)
	}
}
