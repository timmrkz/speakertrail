package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog"

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

// SpeechJob is a download of a model, in the shape of FrameFairy's job,
// so its card reads it the same way.
type SpeechJob struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// queued, running, done, failed or cancelled.
	State    string           `json:"state"`
	Progress *speech.Progress `json:"progress,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// SpeechState is what Settings shows: every model, best first, the folder
// they live in with the home folder as ~, and the latest download.
type SpeechState struct {
	Models []SpeechModelView `json:"models"`
	Folder string            `json:"folder"`
	Job    *SpeechJob        `json:"job,omitempty"`
}

// errSpeechLocked is why a model may not go while Dictate hears with it.
var errSpeechLocked = errors.New("Dictate is hearing with it now. Stop dictating first")

type speechModels struct {
	mu  sync.Mutex
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
	// The latest download, and how to stop it while it runs.
	job    *SpeechJob
	cancel context.CancelFunc
	jobs   int
	// done is closed when the running download has ended. Tests wait on it.
	done chan struct{}
}

func newSpeechModels(dir string, told func(SpeechState)) *speechModels {
	return &speechModels{dir: dir, models: speech.Models, told: told}
}

// state is what Settings shows now.
func (s *speechModels) state() SpeechState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked()
}

func (s *speechModels) stateLocked() SpeechState {
	st := SpeechState{Models: []SpeechModelView{}, Folder: tildePath(s.dir)}
	models := s.models()
	using, _ := speech.InUse(models, s.dir)
	for _, m := range models {
		st.Models = append(st.Models, SpeechModelView{Model: m, Installed: m.Installed(s.dir), InUse: m.Name == using.Name})
	}
	if s.job != nil {
		j := *s.job
		st.Job = &j
	}
	return st
}

// tell sends the state to the window. Called without s.mu.
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
// click shows. One download at a time: asked again while one runs, it
// gives the one that runs.
func (s *speechModels) install(name string) SpeechJob {
	s.mu.Lock()
	if s.job != nil && (s.job.State == "running" || s.job.State == "queued") {
		j := *s.job
		s.mu.Unlock()
		return j
	}
	s.jobs++
	id := "speech-" + strconv.Itoa(s.jobs)
	m, ok := s.byName(name)
	if !ok {
		j := SpeechJob{ID: id, Kind: "model", Title: "Speech model", State: "failed", Error: "There is no speech model called " + name}
		s.job = &j
		s.mu.Unlock()
		s.tell()
		return j
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.job = &SpeechJob{ID: id, Kind: "model", Title: m.Title, State: "running",
		Progress: &speech.Progress{Text: "starting", Fraction: speech.Unknown, Remaining: speech.Unknown}}
	s.cancel = cancel
	done := make(chan struct{})
	s.done = done
	j := *s.job
	in := s.in
	in.Report = func(p *speech.Progress) {
		s.mu.Lock()
		if s.job != nil && s.job.ID == id {
			s.job.Progress = p
		}
		s.mu.Unlock()
		s.tell()
	}
	dir := s.dir
	s.mu.Unlock()
	theLog.line(zerolog.InfoLevel, "speech").Str("model", m.Name).Int64("bytes", m.Download).Msg("the speech model is fetched")
	s.tell()
	go func() {
		defer close(done)
		defer cancel()
		err := in.Install(ctx, m, dir)
		s.mu.Lock()
		switch {
		case err == nil:
			s.job.State = "done"
		case errors.Is(err, speech.ErrCancelled):
			s.job.State = "cancelled"
		default:
			s.job.State = "failed"
			s.job.Error = sentence(err.Error())
		}
		s.job.Progress = nil
		s.cancel = nil
		state := s.job.State
		s.mu.Unlock()
		ev := theLog.line(zerolog.InfoLevel, "speech")
		if err != nil && !errors.Is(err, speech.ErrCancelled) {
			ev = theLog.line(zerolog.WarnLevel, "speech").Err(err)
		}
		ev.Str("model", m.Name).Str("state", state).Msg("the speech model's download ended")
		s.tell()
	}()
	return j
}

// stop cancels the running download. What arrived stays, and the next
// Download carries on from it.
func (s *speechModels) stop(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.ID != id || s.cancel == nil {
		return fmt.Errorf("there is no download %s running", id)
	}
	s.cancel()
	return nil
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
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("it is being downloaded. Cancel the download first")
	}
	if s.letGo != nil {
		s.letGo()
	}
	err := speech.Remove(m, s.dir)
	s.mu.Unlock()
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
