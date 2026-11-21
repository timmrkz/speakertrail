//go:build darwin

package keys

import "testing"

// A key saved now lets any build past the list of trusted apps, so only
// the partition list asks a new build, once. It is built in memory, so
// the test needs no keychain and puts no box on screen.
func TestANewKeyIsAskedForOnce(t *testing.T) {
	if !accessIsOpen() {
		t.Error("a new key keeps a list of trusted apps, so a new build would be asked twice")
	}
}
