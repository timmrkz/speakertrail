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
// Loading the model takes llama-server 7 to 24 seconds on an M2 Max. Tim
// drafts one message after another, so the server is kept for a while
// after a draft, and the next draft finds the model loaded. The app keeps
// it from its start until it quits, see Always and Warm. One model runs
// at a time, because two of them do not fit in the memory of a Mac. As
// FrameFairy's modelhost.go keeps its model between searches, with less to
// it: no search reads ahead into it, and no warm-up waits for one.

// Keep is how long a model nobody holds stays loaded, unless the host
// keeps it always. Metal keeps its memory for llama-server while it is
// loaded, see freemem.go.
const Keep = 10 * time.Minute

// ErrClosed is a model asked for after the app began to quit.
var ErrClosed = errors.New("the app is quitting")

// errDropped is a model dropped while it loaded.
var errDropped = errors.New("the language model was removed while it loaded")

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
	// Notes is the folder a server the app starts is written down in
	// while it runs, see leftover.go. Empty writes nothing.
	Notes string
	// Always keeps a model loaded once it is, after every draft, until it
	// is dropped, another takes its place or the app quits, rather than
	// for Keep. Tim chose it on 21 November 2026, with the model loaded as
	// the app starts, see Warm, so the first draft writes at once. A
	// server that stopped answering still goes at once.
	Always bool

	// waits is told when Warm waits for a draft to let go, for a test.
	waits func()

	mu      sync.Mutex
	server  *Server
	model   string
	users   int
	callOff func() bool
	// loading is closed when the load under way ends, nil when none is.
	// loadingModel is the model it loads, and dropped says it was dropped
	// meanwhile, so it is stopped as soon as it is up.
	loading      chan struct{}
	loadingModel string
	dropped      bool
	cancel       context.CancelFunc
	closed       bool
	// free is closed when the last holder lets go, for Warm to wait on,
	// nil while nobody waits.
	free chan struct{}
}

func (h *Host) launch(ctx context.Context, model string) (*Server, error) {
	if h.Launch != nil {
		return h.Launch(ctx, model)
	}
	server, err := FindServer()
	if err != nil {
		return nil, err
	}
	// None of this app's own servers runs now, so any running holds
	// memory somebody else started it for.
	if err := roomFor(modelNeeds(model), FreeMemory(), runningServers()); err != nil {
		return nil, err
	}
	s, err := Start(ctx, server, model, h.Client, h.Log)
	if err != nil {
		return nil, err
	}
	noteServer(h.Notes, serverNote{PID: s.PID, Port: s.Port, Model: model})
	stop := s.stop
	s.stop = func() {
		stop()
		forgetServer(h.Notes, s.PID)
	}
	return s, nil
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
// the wait it names, Keep after a draft, after which the server stops. A
// wait of 0 stops it at once: a server that stopped answering is let go
// of that way, so the next draft loads one that works, as in FrameFairy.
func (h *Host) Hold(ctx context.Context, model string) (*Server, func(keep time.Duration), error) {
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
		h.loading, h.loadingModel, h.dropped, h.cancel = done, model, false, cancel
		h.mu.Unlock()

		s, err := h.launch(loadCtx, model)
		cancel()

		h.mu.Lock()
		dropped := h.dropped
		h.loading, h.loadingModel, h.dropped, h.cancel = nil, "", false, nil
		close(done)
		if err == nil && h.closed {
			s.Stop()
			s, err = nil, ErrClosed
		}
		if err == nil && dropped {
			s.Stop()
			s, err = nil, errDropped
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
func (h *Host) releaser(s *Server) func(keep time.Duration) {
	var once sync.Once
	return func(keep time.Duration) {
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
			if h.free != nil {
				close(h.free)
				h.free = nil
			}
			if keep <= 0 {
				h.shutLocked()
				return
			}
			if h.Always {
				return
			}
			h.callOff = h.idle(keep, func() {
				h.mu.Lock()
				defer h.mu.Unlock()
				if h.server == s && h.users == 0 {
					h.shutLocked()
				}
			})
		})
	}
}

// Warm loads the model at path, or finds it loaded, and lets go of it at
// once, so a draft that comes later finds it loaded. With Always set it
// then stays.
//
// Another model a draft still writes with is waited for, so a model
// picked meanwhile is loaded once that draft lets go, not by the next.
func (h *Host) Warm(ctx context.Context, model string) error {
	for {
		_, release, err := h.Hold(ctx, model)
		if errors.Is(err, errBusy) {
			if err := h.waitFree(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		release(Keep)
		return nil
	}
}

// waitFree waits until nobody holds the model loaded now.
func (h *Host) waitFree(ctx context.Context) error {
	h.mu.Lock()
	if h.users == 0 || h.server == nil {
		h.mu.Unlock()
		return nil
	}
	if h.free == nil {
		h.free = make(chan struct{})
	}
	free := h.free
	h.mu.Unlock()
	if h.waits != nil {
		h.waits()
	}
	select {
	case <-free:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Drop stops the server with the model at path, unless somebody holds
// it, so its file can be removed. It says false when it is held.
//
// A model still loading is called off, and stopped should it come up
// anyway, so a model removed while it loads is never left loaded.
func (h *Host) Drop(model string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loading != nil && h.loadingModel == model {
		h.dropped = true
		h.cancel()
	}
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
	// A Warm waiting for a draft to let go waits no more, and finds the
	// host closed.
	if h.free != nil {
		close(h.free)
		h.free = nil
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
