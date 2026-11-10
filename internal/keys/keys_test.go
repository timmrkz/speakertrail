package keys

import (
	"sync"
	"testing"
)

type memory struct {
	mu     sync.Mutex
	secret string
	reads  int
}

func (m *memory) has() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.secret != "" }
func (m *memory) get() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	return m.secret, m.secret != ""
}
func (m *memory) set(s string) error { m.mu.Lock(); defer m.mu.Unlock(); m.secret = s; return nil }
func (m *memory) remove()            { m.mu.Lock(); defer m.mu.Unlock(); m.secret = "" }

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

// Every search of a run asks for the key, from several goroutines.
func TestTheKeyIsReadOnceFromSeveralGoroutines(t *testing.T) {
	m := &memory{secret: "kept"}
	e := &Exa{v: m, env: func(string) string { return "" }}
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
}

func TestNoKeychainSaysSo(t *testing.T) {
	e := &Exa{v: nowhere{}, env: func(string) string { return "" }}
	if err := e.Set("a-key"); err != ErrNoKeychain {
		t.Errorf("err = %v", err)
	}
}
