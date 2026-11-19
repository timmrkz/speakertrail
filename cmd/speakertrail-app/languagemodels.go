package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

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
	Job    *ModelJob           `json:"job,omitempty"`
}

type languageModels struct {
	dir    string
	models func() []language.Model
	memory int64
	in     language.Installer
	told   func(LanguageState)
	host   *language.Host
	// get is the latest download and how to stop it, see
	// modeldownloads.go.
	get *downloads
}

func newLanguageModels(dir string, host *language.Host, told func(LanguageState)) *languageModels {
	s := &languageModels{dir: dir, models: language.Models, memory: language.MachineMemory(), host: host, told: told}
	s.get = &downloads{kind: "language", tell: s.tell}
	return s
}

func (s *languageModels) state() LanguageState {
	models := s.models()
	_, serverErr := language.FindServer()
	st := LanguageState{Models: []LanguageModelView{}, Folder: tildePath(s.dir), Server: serverErr == nil}
	using, _ := language.InUse(models, s.dir)
	offered, _ := language.Recommended(models, s.memory)
	for _, m := range models {
		st.Models = append(st.Models, LanguageModelView{Model: m, Installed: m.Installed(s.dir), InUse: m.Name == using.Name,
			Recommended: m.Name == offered.Name, Fit: m.FitsIn(s.memory)})
	}
	st.Job = s.get.latest()
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

// install starts fetching a model and gives the job at once, see
// downloads.start.
func (s *languageModels) install(name string) ModelJob {
	m, ok := language.ByName(s.models(), name)
	if !ok {
		return s.get.start(name, "", 0, nil)
	}
	in, dir := s.in, s.dir
	return s.get.start(m.Name, m.Title, m.Download, func(ctx context.Context, report func(*fetch.Progress)) error {
		in.Report = report
		return in.Install(ctx, m, dir)
	})
}

func (s *languageModels) stop(id string) error {
	return s.get.stop(id)
}

// remove takes a model off the Mac. Not while a draft writes with it, and
// not while it is being fetched.
func (s *languageModels) remove(name string) error {
	m, ok := language.ByName(s.models(), name)
	if !ok {
		return fmt.Errorf("there is no language model called %s", name)
	}
	err := s.get.whileIdle(func() error {
		if s.host != nil && !s.host.Drop(filepath.Join(s.dir, m.Name)) {
			return errors.New("a draft is being written with it now. Wait for it to finish")
		}
		return language.Remove(m, s.dir)
	})
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
