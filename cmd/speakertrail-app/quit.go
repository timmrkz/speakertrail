package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
)

// Ported from FrameFairy's cmd/framefairy-app/quit.go at abb9007, issue
// #17. What differs: the question is taken away over POST /api/quit/stay
// rather than a Wails binding, and closeDown stops what Speaker Trail
// starts, a run and a build waiting to go in place.

// quitAgain is how long the second Cmd+Q has to come after the first. The
// interface shows the question for as long, QUIT_AGAIN_MS in
// web/src/lib/api.ts, which TestTheQuestionLastsAsLongAsTheSecondPressMayCome
// holds equal to this.
const quitAgain = 3 * time.Second

// leaving decides what Cmd+Q, Quit in the menu and closing the app's window
// do, and it is where Wails asks, ShouldQuit.
//
// Stopping everything the app started takes a moment: a run has to let
// its checks end. On macOS Wails runs its shutdown hooks on the main
// thread, so doing it there froze the app with the spinning wheel for as
// long as it took, and nothing said the key had been heard. So the first
// answer is always no. The app says at once what is happening, stops
// everything on a goroutine of its own, and then quits for real, which
// Wails asks about again and this time hears yes.
//
// The first Cmd+Q only asks, the way Chrome does, and the second within
// quitAgain quits. Always, on every page, whether work runs or not. In
// FrameFairy it asked only while work ran, so one page took two presses
// and another quit on the first, and a key that does not do the same
// thing twice is a key nobody can trust.
type leaving struct {
	mu      sync.Mutex
	askedAt time.Time
	going   bool
	gone    bool
	// windowGone is set once the app's window is closing. Nobody can press
	// Cmd+Q a second time into a window that is no longer there.
	windowGone bool

	// asked says whether the quitting was asked for in the app already, by
	// Relaunch on the Updates page, which quits into the new build and has
	// nothing to ask about.
	asked func() bool
	// say tells the interface: "ask" for the question, "going" once the app
	// is on its way out.
	say func(string)
	// stop stops everything the app started, and quit quits for real.
	stop func()
	quit func()
}

func (l *leaving) shouldQuit() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.gone:
		return true
	case l.going:
		return false
	case !l.windowGone && !l.asked() && time.Since(l.askedAt) > quitAgain:
		l.askedAt = time.Now()
		l.say("ask")
		return false
	}
	l.going = true
	l.say("going")
	go func() {
		l.stop()
		l.mu.Lock()
		l.gone = true
		l.mu.Unlock()
		l.quit()
	}()
	return false
}

// stay is the question taken away, with Escape or a click. The next Cmd+Q
// asks again. In FrameFairy it did not at first: the question went from
// the screen and the press still counted, so Cmd+Q, Escape, a click on
// another page and Cmd+Q there quit on what looked like the first press.
func (l *leaving) stay() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.going {
		l.askedAt = time.Time{}
	}
}

// closing is the app's window on its way. The app quits after it without
// asking.
func (l *leaving) closing() {
	l.mu.Lock()
	l.windowGone = true
	l.mu.Unlock()
}

// closeDown is the one way the app stops everything it started, on the
// way out: a run that goes is stopped, and what it did not get to waits
// for the next run, and a build that is ready goes in place once the app
// is gone, see installOnQuit. main runs it once, as the shutdown hook and
// as what Cmd+Q stops. A part that is to stop on the way out stops here.
func closeDown(e *engine.Engine, c *updating) {
	if id, _ := e.Going(); id != 0 {
		e.Stop(id)
	}
	c.installOnQuit()
}

// quitAPI answers the interface about quitting, at /api/quit/.
//
//	POST /api/quit/stay   the question Cmd+Q asked taken away, with Escape or a click
//
// The question itself goes to the interface as the Wails event called
// quit, see main.go.
type quitAPI struct{ leave *leaving }

func (q quitAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method+" "+r.URL.Path != "POST /api/quit/stay" {
		answer(w, http.StatusNotFound, map[string]string{"error": "No such endpoint"})
		return
	}
	q.leave.stay()
	w.WriteHeader(http.StatusNoContent)
}
