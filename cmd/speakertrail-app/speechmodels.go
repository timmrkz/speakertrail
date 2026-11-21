package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/speech"
)

// The speech models in Settings: which can be fetched, which are here,
// fetching one with Download and stopping it with Cancel, and removing
// one again. FrameFairy runs a download as a job in its job list, and its
// card follows the job. Speaker Trail has no job list, so the one
// download there can be is kept here, in the shape of FrameFairy's job,
// and sent as the event speech whenever it changes, the way updates are
// sent. Ported from FrameFairy's setup.go, InstallSpeechModel and
// RemoveSpeechModel, and its Settings, Speech model.

// SpeechModelView is one model as Settings sees it: the model and
// whether it is here.
type SpeechModelView struct {
	speech.Model
	Installed bool `json:"installed"`
	// InUse marks the one Dictate hears with, see speech.InUse, as
	// FrameFairy's Settings marks the model a search runs on.
	InUse bool `json:"inUse"`
}

// SpeechState is what Settings shows: every model, best first, the folder
// they live in with the home folder as ~, and the latest download.
type SpeechState struct {
	Models []SpeechModelView `json:"models"`
	Folder string            `json:"folder"`
	Job    *ModelJob         `json:"job,omitempty"`
}

// errSpeechLocked is why a model may not go while Dictate hears with it.
var errSpeechLocked = errors.New("Dictate is hearing with it now. Stop dictating first")

type speechModels struct {
	dir string
	// models is every model there is, speech.Models in the app.
	models func() []speech.Model
	// in fetches, with the progress set by start.
	in speech.Installer
	// told hears the state each time it changes, the event speech.
	told func(SpeechState)
	// hearing says whether Dictate holds the model now, and letGo closes
	// the model kept loaded, so its files can go. See speech.Host.
	hearing func() bool
	letGo   func()
	// get is the latest download and how to stop it, see
	// modeldownloads.go.
	get *downloads
}

func newSpeechModels(dir string, told func(SpeechState)) *speechModels {
	s := &speechModels{dir: dir, models: speech.Models, told: told}
	s.get = &downloads{kind: "speech", tell: s.tell}
	return s
}

// state is what Settings shows now.
func (s *speechModels) state() SpeechState {
	st := SpeechState{Models: []SpeechModelView{}, Folder: tildePath(s.dir)}
	models := s.models()
	using, _ := speech.InUse(models, s.dir)
	for _, m := range models {
		st.Models = append(st.Models, SpeechModelView{Model: m, Installed: m.Installed(s.dir), InUse: m.Name == using.Name})
	}
	st.Job = s.get.latest()
	return st
}

// tell sends the state to the window.
func (s *speechModels) tell() {
	if s.told != nil {
		s.told(s.state())
	}
}

// byName is the model of that name.
func (s *speechModels) byName(name string) (speech.Model, bool) {
	return speech.ByName(s.models(), name)
}

// install starts fetching a model, and gives the job at once, so the
// click shows.
func (s *speechModels) install(name string) ModelJob {
	m, ok := s.byName(name)
	if !ok {
		return s.get.start(name, "", 0, nil)
	}
	in, dir := s.in, s.dir
	return s.get.start(m.Name, m.Title, m.Download, func(ctx context.Context, report func(*fetch.Progress)) error {
		in.Report = report
		return in.Install(ctx, m, dir)
	})
}

// stop cancels the running download, see downloads.stop.
func (s *speechModels) stop(id string) error {
	return s.get.stop(id)
}

// remove takes a model off the Mac. Not while Dictate hears with it, and
// not while it is being fetched.
func (s *speechModels) remove(name string) error {
	m, ok := s.byName(name)
	if !ok {
		return fmt.Errorf("there is no speech model called %s", name)
	}
	if s.hearing != nil && s.hearing() {
		return errSpeechLocked
	}
	err := s.get.whileIdle(func() error {
		if s.letGo != nil {
			s.letGo()
		}
		return speech.Remove(m, s.dir)
	})
	theLog.line(zerolog.InfoLevel, "speech").Str("model", m.Name).AnErr("error", err).Msg("the speech model was removed")
	s.tell()
	return err
}

// inUse is the folder of the model Dictate hears with, "" when none is
// here.
func (s *speechModels) inUse() (speech.Model, string) {
	m, ok := speech.InUse(s.models(), s.dir)
	if !ok {
		return speech.Model{}, ""
	}
	return m, filepath.Join(s.dir, m.Name)
}
