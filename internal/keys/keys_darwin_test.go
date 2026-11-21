//go:build darwin

package keys

import (
	"path/filepath"
	"testing"
)

// A key saved now lets any build past the list of trusted apps, so only
// the partition list asks a new build, once. It is built in memory, so
// the test needs no keychain and puts no box on screen.
func TestANewKeyIsAskedForOnce(t *testing.T) {
	if !accessIsOpen() {
		t.Error("a new key keeps a list of trusted apps, so a new build would be asked twice")
	}
}

// The real keychain, one of the test's own: a key an older build saved is
// read, saved again the new way, and read again, all without a box, which
// is turned off here so that one would fail the read.
func TestAKeySavedTheOldWayIsSavedAgainInTheKeychain(t *testing.T) {
	k, drop, err := testKeychain(filepath.Join(t.TempDir(), "test.keychain"))
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	if k.has() {
		t.Fatal("a new keychain has the key")
	}
	if err := k.setOld("an-invented-key"); err != nil {
		t.Fatal(err)
	}
	if k.open() {
		t.Fatal("a key saved the old way is taken for one saved the new way")
	}
	e := &Exa{v: k, env: func(string) string { return "" }}
	if got := e.Get(); got != "an-invented-key" {
		t.Fatalf("get %q", got)
	}
	if !k.open() {
		t.Error("the key was not saved again the new way")
	}
	if secret, status := k.get(); secret != "an-invented-key" || status != 0 {
		t.Errorf("read again: %q, status %d", secret, status)
	}
	if err := e.Set("another-invented-key"); err != nil {
		t.Fatal(err)
	}
	if secret, status := k.get(); secret != "another-invented-key" || status != 0 || !k.open() {
		t.Errorf("after a save: %q, status %d, open %v", secret, status, k.open())
	}
}
