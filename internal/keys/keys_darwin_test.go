//go:build darwin

package keys

import (
	"os"
	"path/filepath"
	"testing"
)

// The real keychain, one of the test's own: a key an older build saved
// there is moved to the file and deleted from the keychain, all without a
// box, which is turned off here so that one would fail the read.
func TestAKeyInTheKeychainIsMovedToItsFile(t *testing.T) {
	k, drop, err := testKeychain(filepath.Join(t.TempDir(), "test.keychain"))
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	if k.has() {
		t.Fatal("a new keychain has the key")
	}
	if err := k.put("an-invented-key"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	e := &Exa{path: filepath.Join(dir, File), old: k, env: func(string) string { return "" }}
	if !e.Has() {
		t.Error("a key in the keychain is not there")
	}
	if got, err := e.Get(); got != "an-invented-key" || err != nil {
		t.Fatalf("get %q, %v", got, err)
	}
	if k.has() {
		t.Error("the key is still in the keychain")
	}
	if body, err := os.ReadFile(filepath.Join(dir, File)); err != nil || string(body) != "an-invented-key\n" {
		t.Errorf("the file holds %q, %v", body, err)
	}
}
