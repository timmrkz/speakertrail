package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/rs/zerolog"

	"github.com/timmrkz/speakertrail/internal/fetch"
)

// The download of a model in Settings, the speech model's and the
// language model's alike: one at a time, answered at once so a click
// shows, told to the window as it comes, and stopped with Cancel. One
// controller for both, as FrameFairy draws both with one ModelCard. See
// speechmodels.go and languagemodels.go for what differs.

// ModelJob is a download of a model, in the shape of FrameFairy's job,
// so its card reads it the same way.
type ModelJob struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// queued, running, done, failed or cancelled.
	State    string          `json:"state"`
	Progress *fetch.Progress `json:"progress,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// downloads holds the latest download of one kind of model.
type downloads struct {
	// kind is speech or language, in the ids and the log.
	kind string
	// tell sends the state to the window. Called without mu.
	tell func()

	mu     sync.Mutex
	job    *ModelJob
	cancel context.CancelFunc
	jobs   int
	// done is closed when the running download has ended. Tests wait on it.
	done chan struct{}
}

// latest is a copy of the latest download, nil before the first.
func (d *downloads) latest() *ModelJob {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.job == nil {
		return nil
	}
	j := *d.job
	return &j
}

// start fetches the model with install and gives the job at once. One
// download at a time: asked again while one runs, it gives the one that
// runs. title is "" for a model there is none of, which fails at once.
func (d *downloads) start(name, title string, bytes int64, install func(ctx context.Context, report func(*fetch.Progress)) error) ModelJob {
	d.mu.Lock()
	if d.job != nil && (d.job.State == "running" || d.job.State == "queued") {
		j := *d.job
		d.mu.Unlock()
		return j
	}
	d.jobs++
	id := d.kind + "-" + strconv.Itoa(d.jobs)
	if title == "" {
		j := ModelJob{ID: id, Kind: "model", Title: "Model", State: "failed", Error: "There is no " + d.kind + " model called " + name}
		d.job = &j
		d.mu.Unlock()
		d.told()
		return j
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.job = &ModelJob{ID: id, Kind: "model", Title: title, State: "running",
		Progress: &fetch.Progress{Text: "starting", Fraction: fetch.Unknown, Remaining: fetch.Unknown}}
	d.cancel = cancel
	done := make(chan struct{})
	d.done = done
	j := *d.job
	d.mu.Unlock()
	report := func(p *fetch.Progress) {
		d.mu.Lock()
		if d.job != nil && d.job.ID == id {
			d.job.Progress = p
		}
		d.mu.Unlock()
		d.told()
	}
	theLog.line(zerolog.InfoLevel, d.kind).Str("model", name).Int64("bytes", bytes).Msg("the " + d.kind + " model is fetched")
	d.told()
	go func() {
		defer close(done)
		defer cancel()
		err := install(ctx, report)
		d.mu.Lock()
		switch {
		case err == nil:
			d.job.State = "done"
		case errors.Is(err, fetch.ErrCancelled):
			d.job.State = "cancelled"
		default:
			d.job.State = "failed"
			d.job.Error = sentence(err.Error())
		}
		d.job.Progress = nil
		d.cancel = nil
		state := d.job.State
		d.mu.Unlock()
		ev := theLog.line(zerolog.InfoLevel, d.kind)
		if err != nil && !errors.Is(err, fetch.ErrCancelled) {
			ev = theLog.line(zerolog.WarnLevel, d.kind).Err(err)
		}
		ev.Str("model", name).Str("state", state).Msg("the " + d.kind + " model's download ended")
		d.told()
	}()
	return j
}

func (d *downloads) told() {
	if d.tell != nil {
		d.tell()
	}
}

// stop cancels the running download. What arrived stays, and the next
// Download carries on from it.
func (d *downloads) stop(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.job == nil || d.job.ID != id || d.cancel == nil {
		return fmt.Errorf("there is no download %s running", id)
	}
	d.cancel()
	return nil
}

// whileIdle runs f while no download runs, and none starts until it is
// done: removing a model, which may not happen while it is fetched.
func (d *downloads) whileIdle(f func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return errors.New("it is being downloaded. Cancel the download first")
	}
	return f()
}

// wait waits for the running download to end, for tests.
func (d *downloads) wait() {
	d.mu.Lock()
	done := d.done
	d.mu.Unlock()
	if done != nil {
		<-done
	}
}
