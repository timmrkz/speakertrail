// Package keys keeps the search provider's API key the way macOS keeps
// secrets, in the keychain, never in a file. The item belongs to the app:
// another program that asks for it gets a box from macOS first, and the
// key never passes through a command line. FrameFairy keeps its keys the
// same way.
//
// EXA_API_KEY in the environment stands in while no key is saved, which
// is how a terminal or a test hands one over. Elsewhere than on a Mac
// there is no keychain, and only the environment counts.
package keys

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// Service is the name of the keychain item, as Keychain Access shows it.
const Service = "Speaker Trail: Exa API key"

// account is the account every item is kept under.
const account = "speakertrail"

// vault is where keys are kept: the keychain on a Mac, memory in tests.
type vault interface {
	has() bool
	get() (string, bool)
	set(secret string) error
	remove()
}

// Exa is the Exa key.
type Exa struct {
	v   vault
	env func(string) string
	// mu keeps the key read once, rather than once per search.
	mu     sync.Mutex
	cached string
	read   bool
}

// NewExa is the Exa key in this machine's keychain.
func NewExa() *Exa { return &Exa{v: system(), env: os.Getenv} }

// Has says whether a key is there, without reading it, so asking never
// puts a box on screen.
func (e *Exa) Has() bool {
	return e.v.has() || e.env("EXA_API_KEY") != ""
}

// Get reads the key, the saved one first.
func (e *Exa) Get() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.read {
		e.cached, _ = e.v.get()
		e.read = true
	}
	if e.cached != "" {
		return e.cached
	}
	return e.env("EXA_API_KEY")
}

// ErrNoKeychain is a machine with nowhere safe to keep a key.
var ErrNoKeychain = errors.New("this machine has no keychain to keep the key in. Set EXA_API_KEY instead")

// Set saves a key, and an empty one removes it.
func (e *Exa) Set(key string) error {
	key = strings.TrimSpace(key)
	if strings.ContainsAny(key, " \t\r\n") || len(key) > 200 {
		return errors.New("that does not look like an Exa key. Copy it from dashboard.exa.ai")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if key == "" {
		e.v.remove()
	} else if err := e.v.set(key); err != nil {
		return err
	}
	e.cached, e.read = key, true
	return nil
}

// nowhere is a machine without a keychain.
type nowhere struct{}

func (nowhere) has() bool           { return false }
func (nowhere) get() (string, bool) { return "", false }
func (nowhere) set(string) error    { return ErrNoKeychain }
func (nowhere) remove()             {}
