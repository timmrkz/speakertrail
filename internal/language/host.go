package language

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// The model kept running between the drafts that want it.
//
// Loading the model takes llama-server 9 to 24 seconds on an M2 Max. Tim
// drafts one message after another, so the server is kept for a while
// after a draft, and the next draft finds the model loaded. One model runs
// at a time, because two of them do not fit in the memory of a Mac. As
// FrameFairy's modelhost.go keeps its model between searches, with less to
// it: no search reads ahead into it, and no warm-up waits for one.

// Keep is how long a model nobody holds stays loaded. A model kept idle
// costs less than its size: llama.cpp maps the weights from the file, and
// the system can take the memory back for another program.
const Keep = 10 * time.Minute

// ErrClosed is a model asked for after the app began to quit.
var ErrClosed = errors.New("the app is quitting")

// errBusy is another model in use.
var errBusy = errors.New("another language model is in use. Wait for its draft to finish")

// Host keeps one llama-server. The zero value is ready, and starts the
// real server.
type Host struct {
	// Log hears when a model loads and stops.
	Log *slog.Logger
	// Client carries every request to the server, see orDefault.
	Client *http.Client
	// Launch starts a server for the model at path, Start with the server
	// FindServer gives when nil. A test puts a stand-in here.
	Launch func(ctx context.Context, model string) (*Server, error)
	// Idle starts the wait a model nobody holds is kept for and gives
	// what calls it off, time.AfterFunc when nil. A test takes the wait
	// in hand itself.
	Idle func(keep time.Duration, gone func()) (callOff func() bool)

	mu      sync.Mutex
	server  *Server
	model   string
	users   int
	callOff func() bool
	// loading is closed when the load under way ends, nil when none is.
	loading chan struct{}
	cancel  context.CancelFunc
	closed  bool
}

func (h *Host) launch(ctx context.Context, model string) (*Server, error) {
	if h.Launch != nil {
		return h.Launch(ctx, model)
	}
	server, err := FindServer()
	if err != nil {
		return nil, err
	}
	return Start(ctx, server, model, h.Client, h.Log)
}

func (h *Host) idle(keep time.Duration, gone func()) func() bool {
	if h.Idle != nil {
		return h.Idle(keep, gone)
	}
	return time.AfterFunc(keep, gone).Stop
}

// Loaded says whether this model is loaded now, so a draft knows whether
// it waits for a load.
func (h *Host) Loaded(model string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.server != nil && h.model == model
}

// Hold gives the server with the model at path loaded, starting it if it
// is not, and the release that lets go of it. The last to let go starts
// the wait of Keep, after which the server stops.
func (h *Host) Hold(ctx context.Context, model string) (*Server, func(), error) {
	for {
		h.mu.Lock()
		switch {
		case h.closed:
			h.mu.Unlock()
			return nil, nil, ErrClosed
		case h.loading != nil:
			wait := h.loading
			h.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
			continue
		case h.server != nil && h.model == model:
			h.users++
			if h.callOff != nil {
				h.callOff()
				h.callOff = nil
			}
			s := h.server
			h.mu.Unlock()
			return s, h.releaser(s), nil
		case h.server != nil && h.users > 0:
			h.mu.Unlock()
			return nil, nil, errBusy
		case h.server != nil:
			h.shutLocked()
		}
		// Nothing loaded, or another model nobody uses, which went above.
		done := make(chan struct{})
		loadCtx, cancel := context.WithCancel(ctx)
		h.loading, h.cancel = done, cancel
		h.mu.Unlock()

		s, err := h.launch(loadCtx, model)
		cancel()

		h.mu.Lock()
		h.loading, h.cancel = nil, nil
		close(done)
		if err == nil && h.closed {
			s.Stop()
			s, err = nil, ErrClosed
		}
		if err != nil {
			h.mu.Unlock()
			return nil, nil, err
		}
		h.server, h.model, h.users = s, model, 1
		h.mu.Unlock()
		return s, h.releaser(s), nil
	}
}

// releaser lets go of s once.
func (h *Host) releaser(s *Server) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.server != s {
				return
			}
			h.users--
			if h.users > 0 {
				return
			}
			h.callOff = h.idle(Keep, func() {
				h.mu.Lock()
				defer h.mu.Unlock()
				if h.server == s && h.users == 0 {
					h.shutLocked()
				}
			})
		})
	}
}

// Drop stops the server with the model at path, unless somebody holds
// it, so its file can be removed. It says false when it is held.
func (h *Host) Drop(model string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server == nil || h.model != model {
		return true
	}
	if h.users > 0 {
		return false
	}
	h.shutLocked()
	return true
}

// Close stops the server, loaded or loading, and loads none after it. The
// app calls it on the way out, so no server holds the memory after it.
func (h *Host) Close() {
	h.mu.Lock()
	h.closed = true
	if h.cancel != nil {
		h.cancel()
	}
	wait := h.loading
	if h.server != nil {
		h.shutLocked()
	}
	h.mu.Unlock()
	if wait != nil {
		<-wait
	}
}

// shutLocked stops the server. h.mu is held.
func (h *Host) shutLocked() {
	if h.callOff != nil {
		h.callOff()
		h.callOff = nil
	}
	s := h.server
	h.server, h.model, h.users = nil, "", 0
	if s != nil {
		s.Stop()
		if h.Log != nil {
			h.Log.Info("the language model was let go of", "pid", s.PID)
		}
	}
}
