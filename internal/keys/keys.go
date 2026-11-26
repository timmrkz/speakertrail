// Package keys keeps the search provider's API key in a file of its own,
// exa-key in the app's folder, that only Tim's account can read. Not in
// the keychain: every build macOS did not know yet asked for the keychain
// password to read it, and typing that password ten times a day puts at
// risk something worth far more than an Exa key (Tim, 26 November 2026).
//
// Builds before this one kept the key in the keychain. The first read
// that finds no file reads it there once, which may ask for the password
// one last time, writes it to the file and deletes it from the keychain,
// whatever macOS answered. From then on the file is there, empty when no
// key came, and a file that is there means the keychain is never read
// again, even should macOS not have deleted the item. A box closed then
// leaves no key, and Settings takes it again.
//
// EXA_API_KEY in the environment stands in while no key is saved, which
// is how a terminal or a test hands one over.
package keys

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// File is the name of the key's file in the app's folder.
const File = "exa-key"

// Service is the name of the keychain item builds before this one kept
// the key in, as Keychain Access shows it.
const Service = "Speaker Trail: Exa API key"

// account is the account that item was kept under.
const account = "speakertrail"

// vault is the keychain the key is moved out of: the real one on a Mac,
// none elsewhere, memory in tests. has asks without reading the secret,
// so it puts no box on screen. get gives the secret and the keychain's
// status, 0 when it was found. remove gives the keychain's status too.
type vault interface {
	has() bool
	get() (string, int)
	remove() int
}

// Exa is the Exa key.
type Exa struct {
	// path is the key's file.
	path string
	// old is the keychain of builds before this one.
	old vault
	env func(string) string
	// mu keeps the file and the keychain to one reader or writer at a
	// time, so the keychain is read once however many searches ask.
	mu     sync.Mutex
	cached string
	// log says where the key came from, never the key.
	log *slog.Logger
}

// NewExa is the Exa key in dir, the app's folder, with what happens to
// it written to log.
func NewExa(dir string, log *slog.Logger) *Exa {
	return &Exa{path: filepath.Join(dir, File), old: system(), env: os.Getenv, log: log}
}

// Has says whether a key is there, without reading the keychain, so
// asking never puts a box on screen. It takes no lock, so Settings never
// waits on a box a run put up.
func (e *Exa) Has() bool {
	if e.env("EXA_API_KEY") != "" {
		return true
	}
	if info, err := os.Stat(e.path); err == nil {
		return info.Size() > 0
	}
	return e.old.has()
}

// ErrNoKey is no key saved, and none in the environment.
var ErrNoKey = errors.New("no Exa key is saved")

// ErrUnreadable is a key's file that is there and cannot be read.
var ErrUnreadable = errors.New("the Exa key's file could not be read. Save the key again in Settings")

// Get reads the key, the saved one first, then EXA_API_KEY. It is
// ErrNoKey when there is neither.
func (e *Exa) Get() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cached == "" {
		key, there, err := e.read()
		if err != nil {
			return "", err
		}
		if !there {
			key = e.moveOut()
		}
		e.cached = key
	}
	if e.cached != "" {
		return e.cached, nil
	}
	if key := e.env("EXA_API_KEY"); key != "" {
		return key, nil
	}
	return "", ErrNoKey
}

// read is the key in the file, and whether the file is there at all. An
// empty file is no key, and says the keychain is done with.
func (e *Exa) read() (key string, there bool, err error) {
	body, err := os.ReadFile(e.path)
	// A folder that is a file holds no file either.
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return "", false, nil
	}
	if err != nil {
		e.logger().Error("the Exa key's file could not be read", "error", err)
		return "", true, ErrUnreadable
	}
	return strings.TrimSpace(string(body)), true, nil
}

// moveOut moves a key an older build kept in the keychain into the file,
// and gives it, or nothing when there was none or macOS kept it back. The
// file is written either way, empty when no key came, so the keychain is
// asked once at most, and the item is deleted.
func (e *Exa) moveOut() string {
	if !e.old.has() {
		return ""
	}
	start := time.Now()
	secret, status := e.old.get()
	e.logger().Info("the Exa key was read from the keychain, to move it to its file", "ms", time.Since(start).Milliseconds(),
		"found", status == 0 && secret != "", "status", status)
	secret = strings.TrimSpace(secret)
	if status != 0 || secret == "" {
		secret = ""
		e.logger().Warn("macOS kept the Exa key back. Save it again in Settings", "status", status)
	}
	if err := e.write(secret); err != nil {
		// The keychain keeps it, so the next start tries again.
		e.logger().Error("the Exa key's file could not be written, so the key stays in the keychain", "error", err)
		return secret
	}
	e.removeOld()
	if secret != "" {
		e.logger().Info("the Exa key was moved from the keychain to its file")
	}
	return secret
}

// removeOld deletes the item from the keychain, and logs a delete macOS
// refused. The file is there by then, so the item is never read again.
func (e *Exa) removeOld() {
	if status := e.old.remove(); status != 0 && status != errItemNotFound {
		e.logger().Warn("the Exa key could not be deleted from the keychain, and it is never read there again", "status", status)
	}
}

// Set saves a key, and an empty one removes it. Removing leaves the file
// empty, so the keychain stays done with.
func (e *Exa) Set(key string) error {
	key = strings.TrimSpace(key)
	if strings.ContainsAny(key, " \t\r\n") || len(key) > 200 {
		return errors.New("that does not look like an Exa key. Copy it from dashboard.exa.ai")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.write(key); err != nil {
		e.logger().Error("the Exa key could not be saved", "error", err)
		return errors.New("the Exa key could not be saved. Try again")
	}
	if e.old.has() {
		e.removeOld()
	}
	e.cached = key
	e.logger().Info("the Exa key was saved in its file", "removed", key == "")
	return nil
}

// write puts the key in a new file only its owner can read, then moves
// it into place, which on one disk is all or nothing.
func (e *Exa) write(key string) error {
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(e.path), ".exa-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if key != "" {
		key += "\n"
	}
	if _, err := tmp.WriteString(key); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), e.path)
}

func (e *Exa) logger() *slog.Logger {
	if e.log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return e.log
}

// errItemNotFound is the keychain's errSecItemNotFound, the status of a
// key that is not there.
const errItemNotFound = -25300

// nowhere is a machine without a keychain.
type nowhere struct{}

func (nowhere) has() bool          { return false }
func (nowhere) get() (string, int) { return "", errItemNotFound }
func (nowhere) remove() int        { return errItemNotFound }
