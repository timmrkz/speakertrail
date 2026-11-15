package main

// Ported from FrameFairy's cmd/framefairy-app/quit_test.go at abb9007,
// with a test each for what Speaker Trail adds: the time the question
// lasts held to quitAgain, and POST /api/quit/stay.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// A leaving that records what it said and whether it stopped and quit.
type leaveProbe struct {
	mu      sync.Mutex
	said    []string
	stopped atomic.Int32
	quit    chan struct{}
}

// asked is quitting asked for in the app already, the way Relaunch does.
func newLeaving(asked bool) (*leaving, *leaveProbe) {
	p := &leaveProbe{quit: make(chan struct{}, 4)}
	l := &leaving{
		asked: func() bool { return asked },
		say: func(what string) {
			p.mu.Lock()
			p.said = append(p.said, what)
			p.mu.Unlock()
		},
		stop: func() { p.stopped.Add(1) },
	}
	l.quit = func() {
		// Quitting for real asks again, the way Wails does.
		if l.shouldQuit() {
			p.quit <- struct{}{}
		}
	}
	return l, p
}

func (p *leaveProbe) words() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.said...)
}

// Cmd+Q answers at once, whatever stopping takes. It used to stop
// everything on the main thread, which froze the app under the spinning
// wheel for five seconds with nothing to say the key had been heard.
//
// Stopping is held until Cmd+Q has answered, so an answer that waited for
// it never comes, on a machine of any speed. In the bubble every goroutine
// is then blocked, so the test fails at once as a deadlock. A bound of
// 50 ms on the answer measured the machine as much as the app.
func TestCmdQAnswersAtOnceAndQuitsWhenStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		hold := make(chan struct{})
		stop := l.stop
		l.stop = func() { <-hold; stop() }
		l.shouldQuit()
		if l.shouldQuit() {
			t.Fatal("quit before anything was stopped")
		}
		close(hold)
		if got := p.words(); len(got) != 2 || got[1] != "going" {
			t.Errorf("the interface heard %v", got)
		}
		<-p.quit
		if p.stopped.Load() != 1 {
			t.Errorf("stopped %d times", p.stopped.Load())
		}
	})
}

// The first Cmd+Q only asks, and the second quits. Always, whether work
// runs or not: it asked only while work ran, so the workspace took two
// presses and the settings page quit on the first.
func TestTheFirstCmdQAlwaysAsks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		if l.shouldQuit() {
			t.Fatal("the first Cmd+Q quit")
		}
		if got := p.words(); len(got) != 1 || got[0] != "ask" {
			t.Fatalf("the interface heard %v", got)
		}
		if p.stopped.Load() != 0 {
			t.Fatal("the first Cmd+Q stopped everything")
		}
		l.shouldQuit()
		<-p.quit
		if got := p.words(); len(got) != 2 || got[1] != "going" {
			t.Errorf("the interface heard %v", got)
		}
	})
}

// A second Cmd+Q long after the first asks again.
func TestALateSecondCmdQAsksAgain(t *testing.T) {
	l, p := newLeaving(false)
	l.shouldQuit()
	l.mu.Lock()
	l.askedAt = time.Now().Add(-2 * quitAgain)
	l.mu.Unlock()
	l.shouldQuit()
	if got := p.words(); len(got) != 2 || got[1] != "ask" {
		t.Errorf("the interface heard %v", got)
	}
}

// Closing the app's window quits without asking: there is no window left to
// press Cmd+Q into a second time.
func TestClosingTheWindowQuitsWithoutAsking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		l.closing()
		l.shouldQuit()
		<-p.quit
	})
}

// Relaunch on the Updates page quits into the new build without asking: it
// was asked for with a click, and the question would stand in its way.
func TestRelaunchQuitsWithoutAsking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(true)
		l.shouldQuit()
		<-p.quit
		if got := p.words(); len(got) != 1 || got[0] != "going" {
			t.Errorf("the interface heard %v", got)
		}
	})
}

// Cmd+Q pressed again and again while the app stops stops it once and
// quits once. Stopping is held until every press has answered, and the
// bubble says when every goroutine the presses started is done.
func TestCmdQPressedOverAndOverStopsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		hold := make(chan struct{})
		stop := l.stop
		l.stop = func() { <-hold; stop() }
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() { l.shouldQuit() })
		}
		wg.Wait()
		close(hold)
		synctest.Wait()
		if n := len(p.quit); n != 1 {
			t.Errorf("quit %d times", n)
		}
		if p.stopped.Load() != 1 {
			t.Errorf("stopped %d times", p.stopped.Load())
		}
	})
}

// Cmd+Q, the question taken away with Escape or a click, and Cmd+Q again
// asks again. The second press counted though the question had gone, so
// Cmd+Q on one page, Escape, and Cmd+Q on the next quit at once.
func TestCmdQAfterTheQuestionWasTakenAwayAsksAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		l.shouldQuit()
		l.stay()
		if l.shouldQuit() {
			t.Fatal("quit")
		}
		if got := p.words(); len(got) != 2 || got[1] != "ask" {
			t.Fatalf("the interface heard %v", got)
		}
		if p.stopped.Load() != 0 {
			t.Fatal("stopped everything")
		}
		// Once on its way out, taking the question away changes nothing.
		l.shouldQuit()
		l.stay()
		<-p.quit
	})
}

// The question lasts on screen as long as the second Cmd+Q may come. The
// interface hides it after QUIT_AGAIN_MS in web/src/lib/api.ts, and
// FrameFairy kept its own 3000 equal to quitAgain by a comment alone.
func TestTheQuestionLastsAsLongAsTheSecondPressMayCome(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "lib", "api.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^export const QUIT_AGAIN_MS = (\d+)$`).FindSubmatch(src)
	if m == nil {
		t.Fatal("web/src/lib/api.ts has no line export const QUIT_AGAIN_MS = <ms>")
	}
	if got, want := string(m[1]), strconv.FormatInt(quitAgain.Milliseconds(), 10); got != want {
		t.Errorf("the interface shows the question for %s ms, the second press may come for %s ms", got, want)
	}
}

// Escape or a click on the question asks POST /api/quit/stay, and the next
// Cmd+Q asks again. Any other request is no endpoint, and takes nothing
// away.
func TestStayOverTheAPIAsksAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, p := newLeaving(false)
		h := quitAPI{leave: l}
		l.shouldQuit()
		for _, c := range []struct {
			method, path string
			code         int
		}{
			{"GET", "/api/quit/stay", http.StatusNotFound},
			{"POST", "/api/quit/other", http.StatusNotFound},
		} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
			if rec.Code != c.code {
				t.Errorf("%s %s: code %d, want %d", c.method, c.path, rec.Code, c.code)
			}
		}
		l.mu.Lock()
		asked := !l.askedAt.IsZero()
		l.mu.Unlock()
		if !asked {
			t.Fatal("a request that is no endpoint took the question away")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/quit/stay", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("POST /api/quit/stay: code %d, want 204", rec.Code)
		}
		if l.shouldQuit() {
			t.Fatal("quit")
		}
		if got := p.words(); len(got) != 2 || got[1] != "ask" {
			t.Errorf("the interface heard %v", got)
		}
	})
}
