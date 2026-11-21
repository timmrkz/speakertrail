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
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Service is the name of the keychain item, as Keychain Access shows it.
const Service = "Speaker Trail: Exa API key"

// account is the account every item is kept under.
const account = "speakertrail"

// vault is where keys are kept: the keychain on a Mac, memory in tests.
// get gives the secret and the keychain's status, 0 when it was found.
type vault interface {
	has() bool
	get() (string, int)
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
	// log says when the keychain was read, how long it took and what it
	// answered, never the key. A read may wait on a password box, and
	// the wait shows how long it was on screen.
	log *slog.Logger
}

// NewExa is the Exa key in this machine's keychain, with each read of it
// written to log.
func NewExa(log *slog.Logger) *Exa {
	return &Exa{v: system(), env: os.Getenv, log: log}
}

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
		start := time.Now()
		secret, status := e.v.get()
		e.cached, e.read = secret, true
		e.logger().Info("the Exa key was read from the keychain", "ms", time.Since(start).Milliseconds(),
			"found", status == 0, "status", status)
	}
	if e.cached != "" {
		return e.cached
	}
	return e.env("EXA_API_KEY")
}

// errItemNotFound is the keychain's errSecItemNotFound, the status of a
// key that is not there.
const errItemNotFound = -25300

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
	e.logger().Info("the Exa key was saved in the keychain", "removed", key == "")
	return nil
}

func (e *Exa) logger() *slog.Logger {
	if e.log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return e.log
}

// nowhere is a machine without a keychain.
type nowhere struct{}

func (nowhere) has() bool          { return false }
func (nowhere) get() (string, int) { return "", errItemNotFound }
func (nowhere) set(string) error   { return ErrNoKeychain }
func (nowhere) remove()            {}
