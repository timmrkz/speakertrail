package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Ported from FrameFairy's chrome_test.go at 57bca0c. Its test of the
// report window has nothing to test here, with one window. Telling the
// interface is tested on publish itself, through the tell it is given,
// and the log through slog rather than FrameFairy's log file.

// A measurement crosses to the interface on whole pixels and never as a
// fraction, because something sitting between two pixels is painted in one
// place normally and another the moment anything puts it on a surface of
// its own.
func TestAMeasurementIsAlwaysAWholePixel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   float64
		want int
	}{
		{0, 0}, {13, 13}, {13.4, 13}, {13.5, 14}, {13.6, 14},
		{52.499, 52}, {52.5, 53},
		// Nothing about the app's window is negative, and one that has not
		// been laid out yet can answer anything at all.
		{-1, 0}, {-0.4, 0},
	}
	for _, c := range cases {
		if got := px(c.in); got != c.want {
			t.Errorf("px(%v) was %d, wanted %d", c.in, got, c.want)
		}
	}
}

// Going in and out of fullscreen is the one case where there is nothing to
// measure: no title bar, no buttons. The bar has to keep the height it had
// or the whole workspace under it jumps twice per trip.
func TestFullscreenKeepsTheBarAndLosesTheButtons(t *testing.T) {
	t.Parallel()
	c := &chromeWatch{}
	normal := chromeRaw{bar: 52, left: 20, right: 92}

	got := c.settle(normal)
	if (got != Chrome{Bar: 52, Left: 20, Right: 92}) {
		t.Fatalf("outside fullscreen gave %+v", got)
	}

	// The event says so before the animation starts, so no frame of it
	// leaks a bar that is shrinking away.
	c.set(true)
	if got := c.settle(chromeRaw{bar: 30, left: 20, right: 92}); got != (Chrome{Bar: 52}) {
		t.Errorf("halfway into fullscreen gave %+v, wanted the bar it had and no buttons", got)
	}
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 52}) {
		t.Errorf("in fullscreen gave %+v, wanted the bar it had and no buttons", got)
	}
	// On the way out the flag is still set, so the bar holds until
	// macOS says it is out.
	if got := c.settle(chromeRaw{bar: 12, left: 20, right: 92}); got != (Chrome{Bar: 52}) {
		t.Errorf("halfway out gave %+v, wanted the bar it had", got)
	}
	c.set(false)
	after := chromeRaw{bar: 52, left: 20, right: 92}
	if got := c.settle(after); got != (Chrome{Bar: 52, Left: 20, Right: 92}) {
		t.Errorf("out again gave %+v", got)
	}

	// A bar that comes back a different size is remembered at the new
	// one, so the next trip holds the right height.
	bigger := chromeRaw{bar: 60, left: 24, right: 100}
	c.settle(bigger)
	c.set(true)
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 60}) {
		t.Errorf("the second trip gave %+v, wanted the bar it had by then", got)
	}
}

// A measurement that says nothing is not allowed to wipe the bar. The app
// answers zero before it has been laid out, and the page keeps whatever
// the stylesheet says rather than collapsing.
func TestAWindowThatCannotAnswerLeavesTheBarAlone(t *testing.T) {
	t.Parallel()
	c := &chromeWatch{}
	if got := c.settle(chromeRaw{}); got != (Chrome{}) {
		t.Errorf("a measurement with nothing to say gave %+v", got)
	}
	c.settle(chromeRaw{bar: 52, left: 20, right: 92})
	c.set(true)
	if got := c.settle(chromeRaw{fullscreen: true}); got != (Chrome{Bar: 52}) {
		t.Errorf("a bar of zero was remembered, gave %+v", got)
	}
}

// Dragging the app by its corner raises one of these a frame and almost
// none of them change anything, so the interface is told only what is new.
func TestTheWindowIsToldOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	answer := chromeRaw{bar: 32, left: 20, right: 92}
	var told []Chrome
	c := &chromeWatch{
		ask: func(*application.WebviewWindow) chromeRaw {
			mu.Lock()
			defer mu.Unlock()
			return answer
		},
		tell: func(now Chrome) {
			mu.Lock()
			defer mu.Unlock()
			told = append(told, now)
		},
	}
	for i := 0; i < 20; i++ {
		c.publish()
	}
	if len(told) != 1 || told[0] != (Chrome{Bar: 32, Left: 20, Right: 92}) {
		t.Fatalf("twenty measurements of an unmoved app told %+v, wanted one", told)
	}
	mu.Lock()
	answer.bar = 28
	mu.Unlock()
	c.publish()
	if len(told) != 2 || told[1] != (Chrome{Bar: 28, Left: 20, Right: 92}) {
		t.Errorf("an app that changed told %+v, wanted the new bar once more", told)
	}
}

// Window events come in on goroutines of their own, while the page asks
// at /api/chrome. A storm of them with a fullscreen trip in the middle
// leaves the interface told what the window is at the end, with nothing
// told twice in a row.
func TestTheWatchIsUsedFromSeveralGoroutinesAtOnce(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var told []Chrome
	c := &chromeWatch{
		ask: func(*application.WebviewWindow) chromeRaw { return chromeRaw{bar: 32, left: 20, right: 92} },
		tell: func(now Chrome) {
			mu.Lock()
			defer mu.Unlock()
			told = append(told, now)
		},
	}
	api := chromeAPI{get: func() *chromeWatch { return c }}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				switch g % 3 {
				case 0:
					c.publish()
				case 1:
					c.set(i%2 == 0)
				default:
					rec := httptest.NewRecorder()
					api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chrome", nil))
					if rec.Code != http.StatusOK {
						t.Errorf("GET /api/chrome answered %d", rec.Code)
					}
				}
			}
		}(g)
	}
	wg.Wait()
	c.set(false)
	c.publish()
	mu.Lock()
	defer mu.Unlock()
	if len(told) == 0 || told[len(told)-1] != (Chrome{Bar: 32, Left: 20, Right: 92}) {
		t.Fatalf("after the storm the interface was last told %+v", told)
	}
	for i := 1; i < len(told); i++ {
		if told[i] == told[i-1] {
			t.Errorf("the interface was told %+v twice in a row", told[i])
		}
	}
}

// The page asks once when it starts. Before the window is made there is
// nothing to measure, and the answer is zeros, which leave the stylesheet
// its own numbers. Afterwards it is the measurement. Only GET is answered.
func TestThePageAsksWhereTheTitleBarIs(t *testing.T) {
	t.Parallel()
	var watch *chromeWatch
	api := chromeAPI{get: func() *chromeWatch { return watch }}
	ask := func(method string) (int, Chrome) {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(method, "/api/chrome", nil))
		var c Chrome
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
				t.Fatalf("the answer is not JSON: %v", err)
			}
		}
		return rec.Code, c
	}
	if code, c := ask(http.MethodGet); code != http.StatusOK || c != (Chrome{}) {
		t.Errorf("before the window, GET answered %d %+v, want 200 and zeros", code, c)
	}
	watch = &chromeWatch{ask: func(*application.WebviewWindow) chromeRaw {
		return chromeRaw{bar: 31.6, left: 19.6, right: 91.4}
	}}
	if code, c := ask(http.MethodGet); code != http.StatusOK || c != (Chrome{Bar: 32, Left: 20, Right: 91}) {
		t.Errorf("with the window, GET answered %d %+v", code, c)
	}
	if code, _ := ask(http.MethodPost); code != http.StatusMethodNotAllowed {
		t.Errorf("POST answered %d, want 405", code)
	}
}

// lockedBuffer is written by whatever logs while a test listens.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) lines() []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, l := range bytes.Split(b.buf.Bytes(), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(l, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// The log says what macOS answered about the title bar, in points with
// their fractions and with the close button's frame, and what the
// interface was told, once for each change. If the bar looks wrong on the
// Mac, this line is the only number from the real window. FrameFairy's
// #399 was read from it. The app's log keeps info and above, so the line
// is info. It listens to the app's log, so it runs alone.
func TestTheLogSaysWhatMacOSAnswered(t *testing.T) {
	was := slog.Default()
	var out lockedBuffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(was) })

	answer := chromeRaw{bar: 29.5, left: 12, right: 72, closeTop: 6, closeBottom: 22}
	c := &chromeWatch{ask: func(*application.WebviewWindow) chromeRaw { return answer }}
	c.publish()
	c.publish()
	var said []map[string]any
	for _, l := range out.lines() {
		if l["msg"] == "the title bar was measured" {
			said = append(said, l)
		}
	}
	if len(said) != 1 {
		t.Fatalf("two measurements of an unmoved app logged %d lines, want 1", len(said))
	}
	want := map[string]any{
		"level": "INFO", "part": "window", "event": "chrome",
		"bar": 29.5, "left": 12.0, "right": 72.0, "close_top": 6.0, "close_bottom": 22.0, "fullscreen": false,
	}
	for k, v := range want {
		if said[0][k] != v {
			t.Errorf("the line says %s %v, want %v", k, said[0][k], v)
		}
	}
	told, _ := said[0]["told"].(map[string]any)
	if told["bar"] != 30.0 || told["left"] != 12.0 || told["right"] != 72.0 {
		t.Errorf("the line says the interface was told %v, want bar 30, left 12, right 72", said[0]["told"])
	}
}
