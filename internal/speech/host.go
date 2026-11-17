package speech

import (
	"context"
	"sync"

	"github.com/timmrkz/speakertrail/internal/wake"
)

// Host is the one speech model the app keeps loaded: loaded once, shared
// by every dictation, and kept while nobody dictates when it is the one
// Keep named. One model is in memory at a time, never two: a dictation
// that needs another waits for the one in use to be let go of. Loading
// Parakeet takes a few seconds, and the first words of a dictation would
// wait that long each time. Ported from FrameFairy's engine/speechhost.go,
// which shares it between its transcriptions and Report a Problem.
type Host struct {
	// Open loads the model in a folder, asr.OpenOne in the app. A test
	// hands in its own.
	Open func(dir string) (Recognizer, error)

	mu sync.Mutex
	// dir is the folder of the model loaded or loading, rec the model,
	// nil while none is loaded, and users how many dictations hold it or
	// wait for its load.
	dir   string
	rec   Recognizer
	users int
	// loading is set while the model in dir loads.
	loading bool
	// changed wakes whoever waits whenever a model is let go of, loaded,
	// or failed to load, so they look again.
	changed wake.Wake
	// kept is the folder of the model that stays loaded when nobody holds
	// it, see Keep, empty for none.
	kept string
	// closed is set when the app quits. No model is kept after it.
	closed bool
}

// Speech hands out the speech model in dir, loaded the first time it is
// asked for and shared after that, and the release a dictation calls when
// it is done with it. A model nobody holds goes, unless it is the one
// kept. While another model is in use it waits for that one to be let go
// of.
func (h *Host) Speech(ctx context.Context, dir string) (Recognizer, func(), error) {
	h.mu.Lock()
	for {
		if h.dir == dir && (h.rec != nil || h.loading) {
			break
		}
		if h.users == 0 && !h.loading {
			// Nobody holds the model there is, which goes for this one.
			h.closeLocked()
			h.dir, h.loading = dir, true
			h.users++
			h.mu.Unlock()
			rec, err := h.Open(dir)
			h.mu.Lock()
			h.loading = false
			if err != nil {
				h.users--
				h.dir = ""
				h.changed.WakeLocked()
				h.mu.Unlock()
				return nil, nil, err
			}
			h.rec = rec
			h.changed.WakeLocked()
			h.mu.Unlock()
			return rec, h.releaser(), nil
		}
		if !h.waitLocked(ctx) {
			return nil, nil, ctx.Err()
		}
	}
	// The model asked for, loaded or on its way.
	h.users++
	for h.loading {
		if !h.waitLocked(ctx) {
			h.release()
			return nil, nil, ctx.Err()
		}
	}
	if h.rec == nil || h.dir != dir {
		// Its load failed. This dictation tries again, and says why when
		// it fails as well.
		h.users--
		h.mu.Unlock()
		return h.Speech(ctx, dir)
	}
	rec := h.rec
	h.mu.Unlock()
	return rec, h.releaser(), nil
}

// waitLocked waits for the next change, with h.mu held on the way in and
// on the way out, unless ctx ends first, which it says by false, with
// h.mu let go of.
func (h *Host) waitLocked(ctx context.Context) bool {
	ch := h.changed.WaitLocked()
	h.mu.Unlock()
	select {
	case <-ch:
		h.mu.Lock()
		return true
	case <-ctx.Done():
		return false
	}
}

// releaser is how a dictation lets go of the model it holds, once however
// often it is called.
func (h *Host) releaser() func() {
	var once sync.Once
	return func() { once.Do(h.release) }
}

// release lets go of the model one dictation held. The last to let go of
// a model that is not kept closes it.
func (h *Host) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.users--
	h.dropLocked()
	h.changed.WakeLocked()
}

// dropLocked closes the model when nobody holds it and it is not the one
// kept. h.mu is held.
func (h *Host) dropLocked() {
	if h.users > 0 || (h.dir == h.kept && !h.closed) {
		return
	}
	h.closeLocked()
}

// closeLocked closes the model loaded, which nobody holds. h.mu is held.
func (h *Host) closeLocked() {
	if h.rec != nil {
		h.rec.Close()
		h.rec = nil
		h.dir = ""
	}
}

// Keep keeps the model in dir loaded when nobody dictates, once it has
// been loaded, until it is called again for another or KeepNone, or the
// app quits. Speaker Trail loads it on the first press of Dictate rather
// than when the app opens, since most runs of the app dictate nothing.
func (h *Host) Keep(dir string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.kept = dir
	h.dropLocked()
}

// KeepNone gives up the model kept, and closes it now when nobody
// dictates with it. The app calls it before it removes a model's files.
func (h *Host) KeepNone() {
	h.keepNone(false)
}

// Close gives up the model kept for good, on the way out of the app. A
// dictation that holds it still closes it when it lets go.
func (h *Host) Close() {
	h.keepNone(true)
}

func (h *Host) keepNone(closed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.kept = ""
	h.closed = h.closed || closed
	h.dropLocked()
}

// Held says whether a dictation holds a model now, so it is not removed
// under it.
func (h *Host) Held() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.users > 0
}
