package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/language"
)

// Download in Settings fetches the language model, answers at once, and
// the model is then the one that drafts. Remove takes it away again, but
// not while a draft holds it.
//
// On the real disk: the model is a file llama-server reads.
func TestTheLanguageModelInSettings(t *testing.T) {
	data := []byte("GGUF" + strings.Repeat("weights", 100))
	sum := sha256.Sum256(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	m := language.Model{Name: "probe.gguf", Title: "Probe", URL: srv.URL, Download: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	host := &language.Host{Launch: func(context.Context, string) (*language.Server, error) { return &language.Server{}, nil }}
	defer host.Close()
	var told []LanguageState
	s := newLanguageModels(t.TempDir(), host, func(st LanguageState) { told = append(told, st) })
	s.models = func() []language.Model { return []language.Model{m} }
	if _, _, err := s.inUse(); err == nil {
		t.Error("a model drafts before it is here")
	}
	j := s.install("probe.gguf")
	if j.State != "running" {
		t.Errorf("the download did not start at once: %+v", j)
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	<-done
	st := s.state()
	if st.Job.State != "done" || !st.Models[0].Installed || !st.Models[0].InUse {
		t.Errorf("after the download: %+v", st)
	}
	if got, path, err := s.inUse(); err != nil || got.Name != m.Name || path != filepath.Join(s.dir, m.Name) {
		t.Errorf("in use %v %s %v", got.Name, path, err)
	}
	_, release, _ := host.Hold(t.Context(), filepath.Join(s.dir, m.Name))
	if err := s.remove("probe.gguf"); err == nil {
		t.Error("a model a draft holds was removed")
	}
	release(language.Keep)
	if err := s.remove("probe.gguf"); err != nil || s.state().Models[0].Installed {
		t.Errorf("remove: %v", err)
	}
	if len(told) == 0 {
		t.Error("the window was told nothing")
	}
}
