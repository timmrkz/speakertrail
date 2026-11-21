package keys

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// memory is a keychain in memory. closed is a key saved the old way, and
// saves counts the saves. Like the keychain, a save deletes the key first,
// then refuses the next refuse saves, or keeps it the old way when old is
// set.
type memory struct {
	mu     sync.Mutex
	secret string
	reads  int
	closed bool
	saves  int
	refuse int
	old    bool
}

func (m *memory) has() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.secret != "" }
func (m *memory) get() (string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	if m.secret == "" {
		return "", errItemNotFound
	}
	return m.secret, 0
}
func (m *memory) open() bool { m.mu.Lock(); defer m.mu.Unlock(); return !m.closed }
func (m *memory) set(s string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secret = ""
	if m.refuse > 0 {
		m.refuse--
		return false, errors.New("the keychain did not keep the key, status -25293")
	}
	m.secret, m.closed = s, m.old
	m.saves++
	return !m.old, nil
}
func (m *memory) remove() { m.mu.Lock(); defer m.mu.Unlock(); m.secret = "" }

func TestTheSavedKeyComesBeforeTheEnvironment(t *testing.T) {
	env := map[string]string{"EXA_API_KEY": "from-the-terminal"}
	m := &memory{}
	e := &Exa{v: m, env: func(k string) string { return env[k] }}
	if !e.Has() || e.Get() != "from-the-terminal" {
		t.Errorf("without a saved key: has %v, get %q", e.Has(), e.Get())
	}
	if err := e.Set("  saved-in-settings \n"); err != nil {
		t.Fatal(err)
	}
	if e.Get() != "saved-in-settings" || m.secret != "saved-in-settings" {
		t.Errorf("get %q, kept %q", e.Get(), m.secret)
	}
	if err := e.Set("two words"); err == nil {
		t.Error("a key with a space was taken")
	}
	if err := e.Set(""); err != nil || m.secret != "" || e.Get() != "from-the-terminal" {
		t.Errorf("removing: %v, kept %q, get %q", err, m.secret, e.Get())
	}
	env = map[string]string{}
	if e.Has() {
		t.Error("no key anywhere, and Has says there is one")
	}
}

// Every search of a run asks for the key, from several goroutines. The
// keychain is read once, and the log says so once, never with the key.
func TestTheKeyIsReadOnceFromSeveralGoroutines(t *testing.T) {
	m := &memory{secret: "kept"}
	var logged safeBuffer
	e := &Exa{v: m, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if e.Get() != "kept" {
					t.Error("wrong key")
				}
				_ = e.Has()
			}
		})
	}
	wg.Wait()
	if m.reads != 1 {
		t.Errorf("the keychain was read %d times, want once", m.reads)
	}
	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != 1 || strings.Contains(logged.String(), "kept") {
		t.Fatalf("want one line without the key, got %q", lines)
	}
	var l map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &l); err != nil {
		t.Fatal(err)
	}
	if l["msg"] != "the Exa key was read from the keychain" || l["ms"] == nil || l["found"] != true || l["status"] != float64(0) {
		t.Errorf("line %v", l)
	}
}

// A key that is not in the keychain is logged as not found, with the
// keychain's status.
func TestAMissingKeyIsLoggedAsNotFound(t *testing.T) {
	var logged safeBuffer
	e := &Exa{v: &memory{}, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	if e.Get() != "" {
		t.Fatal("a key from nowhere")
	}
	var l map[string]any
	if err := json.Unmarshal(logged.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if l["found"] != false || l["status"] != float64(errItemNotFound) {
		t.Errorf("line %v", l)
	}
}

// A key an older build saved asked a new build twice. It is saved again
// once, right after the read, from however many searches ask at once,
// and the log says so.
func TestAKeySavedTheOldWayIsSavedAgainOnce(t *testing.T) {
	m := &memory{secret: "kept", closed: true}
	var logged safeBuffer
	e := &Exa{v: m, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if e.Get() != "kept" {
				t.Error("wrong key")
			}
		})
	}
	wg.Wait()
	if m.reads != 1 || m.saves != 1 || m.closed || m.secret != "kept" {
		t.Errorf("reads %d, saves %d, closed %v, kept %q", m.reads, m.saves, m.closed, m.secret)
	}
	if !strings.Contains(logged.String(), "the Exa key was saved again, so a new build asks once") {
		t.Errorf("log %s", logged.String())
	}
}

// A key saved the new way is never saved again.
func TestAKeySavedTheNewWayIsLeftAsItIs(t *testing.T) {
	m := &memory{secret: "kept"}
	e := &Exa{v: m, env: func(string) string { return "" }}
	if e.Get() != "kept" || m.saves != 0 {
		t.Errorf("saves %d", m.saves)
	}
}

// A save deletes the old key before it adds the new one. When the add is
// refused once, it is tried again, and the key is kept.
func TestAKeyRefusedOnceIsSavedOnTheSecondTry(t *testing.T) {
	m := &memory{secret: "kept", closed: true, refuse: 1}
	var logged safeBuffer
	e := &Exa{v: m, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	if e.Get() != "kept" || m.secret != "kept" || m.closed {
		t.Errorf("get %q, kept %q, closed %v", e.Get(), m.secret, m.closed)
	}
	if !strings.Contains(logged.String(), "tried once more") || !strings.Contains(logged.String(), "so a new build asks once") {
		t.Errorf("log %s", logged.String())
	}
}

// When the keychain refuses the key twice, it is gone from the keychain.
// This start still has it, and the log says to save it again, as an error.
func TestAKeyRefusedTwiceIsStillUsedAndTheLogSaysItIsGone(t *testing.T) {
	m := &memory{secret: "kept", closed: true, refuse: 2}
	var logged safeBuffer
	e := &Exa{v: m, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	if e.Get() != "kept" {
		t.Errorf("get %q", e.Get())
	}
	if !strings.Contains(logged.String(), `"level":"ERROR"`) || !strings.Contains(logged.String(), "no longer in the keychain") {
		t.Errorf("log %s", logged.String())
	}
}

// A key the keychain kept only the old way is not said to be kept the
// new way.
func TestAKeyKeptTheOldWayIsLoggedAsSuch(t *testing.T) {
	m := &memory{secret: "kept", closed: true, old: true}
	var logged safeBuffer
	e := &Exa{v: m, env: func(string) string { return "" }, log: slog.New(slog.NewJSONHandler(&logged, nil))}
	if e.Get() != "kept" {
		t.Errorf("get %q", e.Get())
	}
	if !strings.Contains(logged.String(), "saved again the old way") || strings.Contains(logged.String(), "asks once") {
		t.Errorf("log %s", logged.String())
	}
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
func (s *safeBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.b.Bytes())
}

func TestNoKeychainSaysSo(t *testing.T) {
	e := &Exa{v: nowhere{}, env: func(string) string { return "" }}
	if err := e.Set("a-key"); err != ErrNoKeychain {
		t.Errorf("err = %v", err)
	}
}
