package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog"

	"github.com/timmrkz/speakertrail/internal/draft"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/language"
)

// The language models in Settings: which can be fetched, which are here,
// fetching one with Download and stopping it with Cancel, and removing
// one again. The same as the speech models beside it, see speechmodels.go,
// and sent as the event language whenever it changes. Ported from
// FrameFairy's Settings, Clip model.

// LanguageModelView is one model as Settings sees it.
type LanguageModelView struct {
	language.Model
	Installed bool `json:"installed"`
	// InUse marks the one that drafts, see language.InUse.
	InUse bool `json:"inUse"`
	// Recommended marks the one offered to this Mac, the largest it holds
	// comfortably, see language.Recommended.
	Recommended bool `json:"recommended"`
	// Fit is what this Mac can do with it: fits, tight, too big or
	// unknown.
	Fit language.Fit `json:"fit"`
}

// LanguageState is what Settings shows: every model, largest first, the
// folder they live in with the home folder as ~, whether llama-server is
// in the app to run them, and the latest download, in the shape of a
// speech model's.
type LanguageState struct {
	Models []LanguageModelView `json:"models"`
	Folder string              `json:"folder"`
	Server bool                `json:"server"`
	Job    *SpeechJob          `json:"job,omitempty"`
}

type languageModels struct {
	mu     sync.Mutex
	dir    string
	models func() []language.Model
	memory int64
	in     language.Installer
	told   func(LanguageState)
	host   *language.Host
	job    *SpeechJob
	cancel context.CancelFunc
	jobs   int
	done   chan struct{}
}

func newLanguageModels(dir string, host *language.Host, told func(LanguageState)) *languageModels {
	return &languageModels{dir: dir, models: language.Models, memory: language.MachineMemory(), host: host, told: told}
}

func (s *languageModels) state() LanguageState {
	s.mu.Lock()
	defer s.mu.Unlock()
	models := s.models()
	_, serverErr := language.FindServer()
	st := LanguageState{Models: []LanguageModelView{}, Folder: tildePath(s.dir), Server: serverErr == nil}
	using, _ := language.InUse(models, s.dir)
	offered, _ := language.Recommended(models, s.memory)
	for _, m := range models {
		st.Models = append(st.Models, LanguageModelView{Model: m, Installed: m.Installed(s.dir), InUse: m.Name == using.Name,
			Recommended: m.Name == offered.Name, Fit: m.FitsIn(s.memory)})
	}
	if s.job != nil {
		j := *s.job
		st.Job = &j
	}
	return st
}

func (s *languageModels) tell() {
	if s.told != nil {
		s.told(s.state())
	}
}

// inUse is the model that drafts and its file, draft.ErrNoModel when
// none is here.
func (s *languageModels) inUse() (language.Model, string, error) {
	m, ok := language.InUse(s.models(), s.dir)
	if !ok {
		return language.Model{}, "", draft.ErrNoModel
	}
	return m, filepath.Join(s.dir, m.Name), nil
}

// install starts fetching a model and gives the job at once. One download
// at a time: asked again while one runs, it gives the one that runs.
func (s *languageModels) install(name string) SpeechJob {
	s.mu.Lock()
	if s.job != nil && (s.job.State == "running" || s.job.State == "queued") {
		j := *s.job
		s.mu.Unlock()
		return j
	}
	s.jobs++
	id := "language-" + strconv.Itoa(s.jobs)
	m, ok := language.ByName(s.models(), name)
	if !ok {
		j := SpeechJob{ID: id, Kind: "model", Title: "Language model", State: "failed", Error: "There is no language model called " + name}
		s.job = &j
		s.mu.Unlock()
		s.tell()
		return j
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.job = &SpeechJob{ID: id, Kind: "model", Title: m.Title, State: "running",
		Progress: &fetch.Progress{Text: "starting", Fraction: fetch.Unknown, Remaining: fetch.Unknown}}
	s.cancel = cancel
	done := make(chan struct{})
	s.done = done
	j := *s.job
	in := s.in
	in.Report = func(p *fetch.Progress) {
		s.mu.Lock()
		if s.job != nil && s.job.ID == id {
			s.job.Progress = p
		}
		s.mu.Unlock()
		s.tell()
	}
	dir := s.dir
	s.mu.Unlock()
	theLog.line(zerolog.InfoLevel, "language").Str("model", m.Name).Int64("bytes", m.Download).Msg("the language model is fetched")
	s.tell()
	go func() {
		defer close(done)
		defer cancel()
		err := in.Install(ctx, m, dir)
		s.mu.Lock()
		switch {
		case err == nil:
			s.job.State = "done"
		case errors.Is(err, fetch.ErrCancelled):
			s.job.State = "cancelled"
		default:
			s.job.State = "failed"
			s.job.Error = sentence(err.Error())
		}
		s.job.Progress = nil
		s.cancel = nil
		state := s.job.State
		s.mu.Unlock()
		ev := theLog.line(zerolog.InfoLevel, "language")
		if err != nil && !errors.Is(err, fetch.ErrCancelled) {
			ev = theLog.line(zerolog.WarnLevel, "language").Err(err)
		}
		ev.Str("model", m.Name).Str("state", state).Msg("the language model's download ended")
		s.tell()
	}()
	return j
}

func (s *languageModels) stop(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.ID != id || s.cancel == nil {
		return fmt.Errorf("there is no download %s running", id)
	}
	s.cancel()
	return nil
}

// remove takes a model off the Mac. Not while a draft writes with it, and
// not while it is being fetched.
func (s *languageModels) remove(name string) error {
	m, ok := language.ByName(s.models(), name)
	if !ok {
		return fmt.Errorf("there is no language model called %s", name)
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("it is being downloaded. Cancel the download first")
	}
	if s.host != nil && !s.host.Drop(filepath.Join(s.dir, m.Name)) {
		s.mu.Unlock()
		return errors.New("a draft is being written with it now. Wait for it to finish")
	}
	err := language.Remove(m, s.dir)
	s.mu.Unlock()
	theLog.line(zerolog.InfoLevel, "language").Str("model", m.Name).AnErr("error", err).Msg("the language model was removed")
	s.tell()
	return err
}

// languageAPI answers Settings about the language models, the way
// speechAPI does about the speech models.
//
//	GET    /api/language                 every model, whether it is here, the folder, the latest download
//	POST   /api/language/install         {"name": "…"} fetches that model, answers the download at once
//	POST   /api/language/cancel          {"id": "language-1"} stops that download, what arrived stays
//	DELETE /api/language/models/{name}   takes that model off the Mac, after Settings asked
type languageAPI struct {
	s *languageModels
}

func (a languageAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	modelsAPI{
		base:    "/api/language",
		state:   func() any { return a.s.state() },
		install: func(name string) any { return a.s.install(name) },
		stop:    a.s.stop,
		remove:  a.s.remove,
	}.ServeHTTP(w, r)
}
