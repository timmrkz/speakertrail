// Package keys keeps the search provider's API key the way macOS keeps
// secrets, in the keychain, never in a file. Each read by a build of the
// app that macOS does not know yet asks for the password once, and Always
// Allow keeps that build from being asked again. Another program that
// asks for it gets a box from macOS first, and the key never passes
// through a command line. See keys_darwin.go for how macOS checks a read.
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
// open says whether the saved key is kept the way set keeps it, so that a
// new build of the app reading it is asked one question, not two, and set
// says whether it kept it that way. See keys_darwin.go.
type vault interface {
	has() bool
	get() (string, int)
	open() bool
	set(secret string) (bool, error)
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

// Get reads the key, the saved one first. A key saved by an older build
// of the app, which asked twice on each read by a new build, is saved
// again right after the read the person has just answered, so the next
// build asks once.
func (e *Exa) Get() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.read {
		start := time.Now()
		secret, status := e.v.get()
		e.cached, e.read = secret, true
		e.logger().Info("the Exa key was read from the keychain", "ms", time.Since(start).Milliseconds(),
			"found", status == 0, "status", status)
		if secret != "" && !e.v.open() {
			e.saveAgain(secret)
		}
	}
	if e.cached != "" {
		return e.cached
	}
	return e.env("EXA_API_KEY")
}

// saveAgain saves a key kept the old way the new way, and says in the log
// how it went. Saving deletes the old item first, so a save that fails may
// leave no key in the keychain. It is then tried once more with the key
// this start still holds.
func (e *Exa) saveAgain(secret string) {
	for try := 1; ; try++ {
		open, err := e.v.set(secret)
		switch {
		case err == nil && open:
			e.logger().Info("the Exa key was saved again, so a new build asks once")
			return
		case err == nil:
			e.logger().Warn("the Exa key was saved again the old way, so a new build still asks twice")
			return
		case try < 2:
			e.logger().Warn("the Exa key could not be saved again, so it is tried once more", "error", err)
		case e.v.has():
			e.logger().Warn("the Exa key could not be saved again, so a new build still asks twice", "error", err)
			return
		default:
			e.logger().Error("the Exa key is no longer in the keychain. Save it again in Settings", "error", err)
			return
		}
	}
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
	open := true
	if key == "" {
		e.v.remove()
	} else if kept, err := e.v.set(key); err != nil {
		return err
	} else {
		open = kept
	}
	e.cached, e.read = key, true
	if open {
		e.logger().Info("the Exa key was saved in the keychain", "removed", key == "")
	} else {
		e.logger().Warn("the Exa key was saved in the keychain the old way, so a new build asks twice")
	}
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

func (nowhere) has() bool                { return false }
func (nowhere) get() (string, int)       { return "", errItemNotFound }
func (nowhere) open() bool               { return true }
func (nowhere) set(string) (bool, error) { return false, ErrNoKeychain }
func (nowhere) remove()                  {}
