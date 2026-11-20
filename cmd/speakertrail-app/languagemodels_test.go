package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/draft"
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
	s.get.wait()
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

// Picking a model in Settings makes it the one that drafts, downloaded or
// not, the way FrameFairy's list of models does: the list says what is
// chosen, and a draft is refused until it is here rather than written with
// another. Removing it lets the choice go.
func TestPickingTheLanguageModel(t *testing.T) {
	small := language.Model{Name: "small.gguf", Title: "Small"}
	large := language.Model{Name: "large.gguf", Title: "Large"}
	var chosen string
	s := newLanguageModels(t.TempDir(), nil, nil)
	s.models = func() []language.Model { return []language.Model{large, small} }
	s.chosen = func() string { return chosen }
	s.choose = func(name string) error { chosen = name; return nil }
	if err := os.WriteFile(filepath.Join(s.dir, small.Name), []byte("GGUF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m, _, err := s.inUse(); err != nil || m.Name != small.Name {
		t.Fatalf("with nothing picked the model here drafts: %v %v", m.Name, err)
	}
	if err := s.use("large.gguf"); err != nil {
		t.Fatal(err)
	}
	st := s.state()
	if !st.Models[0].InUse || st.Models[1].InUse {
		t.Errorf("the list does not say the one picked: %+v", st.Models)
	}
	if _, _, err := s.inUse(); !errors.Is(err, draft.ErrNoModel) {
		t.Errorf("a draft wrote with another model than the one picked: %v", err)
	}
	if err := s.use("nothing.gguf"); err == nil || chosen != "large.gguf" {
		t.Errorf("a model that is not offered was picked: %v, %s", err, chosen)
	}
	if err := s.use("small.gguf"); err != nil {
		t.Fatal(err)
	}
	if err := s.remove("small.gguf"); err != nil {
		t.Fatal(err)
	}
	if chosen != "" {
		t.Errorf("the choice outlived the model: %q", chosen)
	}
}
