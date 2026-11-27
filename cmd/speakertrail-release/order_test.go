package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// sign signs when the run was made into the entry, as the app checks it,
// and a build made before the one on the release does not replace it. An
// entry there that is older, of another channel or not signed so is
// replaced, since only a signed time says which is newer.
//
// On the real disk: the release tool reads and writes the entries as files.
func TestABuildMadeBeforeTheOneOnTheReleaseDoesNotReplaceIt(t *testing.T) {
	private := release(t)
	public := private.Public().(ed25519.PublicKey)
	at := "2026-11-27T19:00:00Z"
	entry := func(made string) string {
		t.Helper()
		out := signed(t, "main", "main")
		if newer, err := orderEntry(out, made, ""); err != nil || newer != "" {
			t.Fatalf("signing made %s: %q, %v", made, newer, err)
		}
		return out
	}
	now := entry(at)
	b := readBuild(t, now)
	if err := updates.VerifyOrder(public, b); err != nil || !b.Made.Equal(time.Date(2026, 11, 27, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("the entry is not signed as made at %s: %v, %v", at, b.Made, err)
	}

	older := signed(t, "main", "main")
	newer, err := orderEntry(older, "2026-11-27T18:00:00Z", now)
	if err != nil || !strings.Contains(newer, "does not replace it") {
		t.Errorf("an older build was to replace a newer one: %q, %v", newer, err)
	}
	later := signed(t, "main", "main")
	if newer, err := orderEntry(later, "2026-11-27T20:00:00Z", now); err != nil || newer != "" {
		t.Errorf("a newer build was not to replace an older one: %q, %v", newer, err)
	}

	unsigned := filepath.Join(t.TempDir(), "channel-main.json")
	plain := b
	plain.Order = ""
	plain.Made = plain.Made.Add(time.Hour)
	if err := writeJSON(unsigned, plain); err != nil {
		t.Fatal(err)
	}
	other := signed(t, "main", "main")
	if newer, err := orderEntry(other, "2026-11-27T18:00:00Z", unsigned); err != nil || newer != "" {
		t.Errorf("an entry whose time is not signed held a build back: %q, %v", newer, err)
	}
	if newer, err := orderEntry(signed(t, "main", "main"), "2026-11-27T18:00:00Z", filepath.Join(t.TempDir(), "none.json")); err != nil || newer != "" {
		t.Errorf("a channel with no entry yet held a build back: %q, %v", newer, err)
	}
	if _, err := orderEntry(signed(t, "main", "main"), "yesterday", ""); err == nil {
		t.Error("a time that is no time was signed")
	}
	_ = os.Remove(unsigned)
}
