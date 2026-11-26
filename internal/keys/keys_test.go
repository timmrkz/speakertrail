package keys

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// memory is a keychain in memory, where builds before this one kept the
// key. reads counts the reads of the secret, the ones that may ask for
// the password.
type memory struct {
	mu     sync.Mutex
	secret string
	reads  int
	// keptBack is how many reads macOS keeps the key back, with -128, as
	// when the password box is closed.
	keptBack int
}

func (m *memory) has() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.secret != "" }
func (m *memory) get() (string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	if m.keptBack > 0 {
		m.keptBack--
		return "", -128
	}
	if m.secret == "" {
		return "", errItemNotFound
	}
	return m.secret, 0
}
func (m *memory) remove() { m.mu.Lock(); defer m.mu.Unlock(); m.secret = "" }

func (m *memory) count() int { m.mu.Lock(); defer m.mu.Unlock(); return m.reads }

// exa is the key in a folder of the test's own, with m for the keychain
// and env for the environment.
func exa(t *testing.T, m *memory, env map[string]string, log *safeBuffer) *Exa {
	t.Helper()
	e := &Exa{path: filepath.Join(t.TempDir(), File), old: m, env: func(k string) string { return env[k] }}
	if log != nil {
		e.log = slog.New(slog.NewJSONHandler(log, nil))
	}
	return e
}

func TestTheSavedKeyComesBeforeTheEnvironment(t *testing.T) {
	env := map[string]string{"EXA_API_KEY": "from-the-terminal"}
	e := exa(t, &memory{}, env, nil)
	if !e.Has() || key(e) != "from-the-terminal" {
		t.Errorf("without a saved key: has %v, get %q", e.Has(), key(e))
	}
	if err := e.Set("  saved-in-settings \n"); err != nil {
		t.Fatal(err)
	}
	if key(e) != "saved-in-settings" || file(t, e) != "saved-in-settings\n" {
		t.Errorf("get %q, file %q", key(e), file(t, e))
	}
	if err := e.Set("two words"); err == nil {
		t.Error("a key with a space was taken")
	}
	if err := e.Set(""); err != nil || key(e) != "from-the-terminal" {
		t.Errorf("removing: %v, get %q", err, key(e))
	}
	if _, err := os.Stat(e.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the file is still there after removing: %v", err)
	}
	delete(env, "EXA_API_KEY")
	if e.Has() {
		t.Error("no key anywhere, and Has says there is one")
	}
	if k, err := e.Get(); k != "" || !errors.Is(err, ErrNoKey) {
		t.Errorf("a key from nowhere: %q, %v", k, err)
	}
}

// The key's file is for its owner alone, and a save leaves nothing else
// in the folder.
func TestTheFileIsReadOnlyByItsOwner(t *testing.T) {
	e := exa(t, &memory{}, nil, nil)
	if err := e.Set("an-invented-key"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v, want -rw-------", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(e.path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want the key's alone", len(entries))
	}
	// A new start reads it from the file.
	again := &Exa{path: e.path, old: &memory{}, env: func(string) string { return "" }}
	if key(again) != "an-invented-key" {
		t.Errorf("a new start reads %q", key(again))
	}
}

// Tim, 26 November 2026: every start asked for the keychain password. A
// key an older build kept there is read once, moved to the file and
// deleted from the keychain, from however many searches ask at once, and
// no later start reads the keychain again. The log never holds the key.
func TestAKeyInTheKeychainIsMovedOnceAndTheKeychainIsNeverAskedAgain(t *testing.T) {
	m := &memory{secret: "kept-in-the-keychain"}
	var logged safeBuffer
	e := exa(t, m, nil, &logged)
	if !e.Has() || m.count() != 0 {
		t.Fatalf("has %v, and the keychain was read %d times to tell", e.Has(), m.count())
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if key(e) != "kept-in-the-keychain" {
					t.Error("wrong key")
				}
				_ = e.Has()
			}
		})
	}
	wg.Wait()
	if m.count() != 1 || m.has() {
		t.Errorf("the keychain was read %d times, want once, and still has the key: %v", m.count(), m.has())
	}
	if file(t, e) != "kept-in-the-keychain\n" {
		t.Errorf("the file holds %q", file(t, e))
	}
	for range 3 {
		start := &Exa{path: e.path, old: m, env: func(string) string { return "" }}
		if key(start) != "kept-in-the-keychain" || !start.Has() {
			t.Errorf("a later start reads %q", key(start))
		}
	}
	if m.count() != 1 {
		t.Errorf("later starts read the keychain, %d reads in all", m.count())
	}
	if strings.Contains(logged.String(), "kept-in-the-keychain") {
		t.Error("the log holds the key")
	}
	if !strings.Contains(logged.String(), "moved from the keychain to its file") {
		t.Errorf("log %s", logged.String())
	}
}

// A password box closed during the move keeps the key back. The item is
// deleted all the same, so the box is never put up again, and Settings
// takes the key.
func TestAKeyKeptBackIsDeletedAndNeverAskedForAgain(t *testing.T) {
	m := &memory{secret: "kept-in-the-keychain", keptBack: 1}
	var logged safeBuffer
	e := exa(t, m, nil, &logged)
	if k, err := e.Get(); k != "" || !errors.Is(err, ErrNoKey) {
		t.Fatalf("the box closed: %q, %v", k, err)
	}
	if m.has() || e.Has() {
		t.Errorf("the key is still there after the box was closed, keychain %v", m.has())
	}
	if _, err := e.Get(); !errors.Is(err, ErrNoKey) || m.count() != 1 {
		t.Errorf("asked again: %v, %d reads, want 1", err, m.count())
	}
	if !strings.Contains(logged.String(), "Save it again in Settings") || !strings.Contains(logged.String(), `"status":-128`) {
		t.Errorf("log %s", logged.String())
	}
}

// Saving a key in Settings deletes the one in the keychain without
// reading it, so it never asks and is never moved over the new one.
func TestSavingAKeyDeletesTheOneInTheKeychainUnread(t *testing.T) {
	m := &memory{secret: "kept-in-the-keychain"}
	e := exa(t, m, nil, nil)
	if err := e.Set("pasted-in-settings"); err != nil {
		t.Fatal(err)
	}
	if m.has() || m.count() != 0 {
		t.Errorf("keychain still has it %v, read %d times", m.has(), m.count())
	}
	again := &Exa{path: e.path, old: m, env: func(string) string { return "" }}
	if key(again) != "pasted-in-settings" || m.count() != 0 {
		t.Errorf("a new start reads %q, keychain read %d times", key(again), m.count())
	}
}

// A file that cannot be read is said to be so, and the keychain is not
// asked in its place.
func TestAFileThatCannotBeReadSaysSo(t *testing.T) {
	m := &memory{secret: "kept-in-the-keychain"}
	e := exa(t, m, nil, nil)
	if err := os.Mkdir(e.path, 0o700); err != nil {
		t.Fatal(err)
	}
	if k, err := e.Get(); k != "" || err == nil || errors.Is(err, ErrNoKey) || m.count() != 0 {
		t.Errorf("get %q, %v, keychain read %d times", k, err, m.count())
	}
}

// Settings saves while the searches of a run read the key.
func TestTheKeyIsSavedAndReadFromSeveralGoroutines(t *testing.T) {
	e := exa(t, &memory{}, nil, nil)
	if err := e.Set("first"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 20 {
				if i%2 == 0 {
					if err := e.Set("second"); err != nil {
						t.Error(err)
					}
				} else if k := key(e); k != "first" && k != "second" {
					t.Errorf("read %q", k)
				}
			}
		})
	}
	wg.Wait()
	if key(e) != "second" || file(t, e) != "second\n" {
		t.Errorf("get %q, file %q", key(e), file(t, e))
	}
}

func key(e *Exa) string {
	k, _ := e.Get()
	return k
}

func file(t *testing.T, e *Exa) string {
	t.Helper()
	body, _ := os.ReadFile(e.path)
	return string(body)
}

// safeBuffer is a log several goroutines write to.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
