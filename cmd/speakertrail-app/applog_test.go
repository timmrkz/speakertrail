package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// testLog opens the app's log in a folder of the test's own and gives
// what it holds, a line each, read as JSON. A line that is not whole JSON
// fails the test.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func testLog(t *testing.T) (path string, read func() []map[string]any) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "logs", "app.log")
	theLog.open(path)
	t.Cleanup(func() {
		theLog.close()
		theLog.recent.forget()
	})
	return path, func() []map[string]any {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var lines []map[string]any
		scan := bufio.NewScanner(f)
		scan.Buffer(nil, 1<<20)
		for scan.Scan() {
			var line map[string]any
			if err := json.Unmarshal(scan.Bytes(), &line); err != nil {
				t.Fatalf("a line of the log is not whole JSON: %v\n%s", err, scan.Text())
			}
			lines = append(lines, line)
		}
		return lines
	}
}

// close stops writing the log, for the tests.
func (l *appLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.root.Store(nil)
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// forget lets the kept lines go, for the tests.
func (r *recentLines) forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines, r.next = nil, 0
}

// find is the first line whose message starts so, or nil.
func find(lines []map[string]any, msg string) map[string]any {
	for _, l := range lines {
		if s, _ := l["msg"].(string); strings.HasPrefix(s, msg) {
			return l
		}
	}
	return nil
}

// has says whether a line holds every field given, as the log wrote it.
func has(t *testing.T, line map[string]any, want map[string]any) {
	t.Helper()
	if line == nil {
		t.Errorf("no line for %v", want)
		return
	}
	for k, v := range want {
		if fmt.Sprint(line[k]) != fmt.Sprint(v) {
			t.Errorf("%s is %v, want %v, in %v", k, line[k], v, line)
		}
	}
}

// What Wails says goes into the app's log from info up, under wails and
// the start of the app. Its debug lines, one for every request the window
// makes, do not.
func TestWailsWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)
	wails := theLog.slog("wails", zerolog.InfoLevel)
	wails.Debug("Asset Request:", "path", "/api/people")
	wails.Error("the window could not be made", "error", "no display")

	lines := read()
	if find(lines, "Asset Request") != nil {
		t.Error("Wails's debug line was written")
	}
	has(t, find(lines, "the window could not be made"), map[string]any{
		"level": "error", "from": "wails", "start": theLog.start, "error": "no display",
	})
}

// What the engine and the API say through slog goes into the app's log
// from trace up, the trace lines to memory, under their part, each line whole while runs say it from
// several goroutines at once. A trace line goes to memory, not the file.
func TestTheEngineAndTheAPIWriteTheAppLog(t *testing.T) {
	_, read := testLog(t)
	engine := theLog.slog("engine", zerolog.TraceLevel)
	api := theLog.slog("api", zerolog.TraceLevel)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			engine.Info("a run started", "run", i, "searches", 3)
			api.Error("a request failed", "path", "/api/runs", "error", "the run is going")
		})
	}
	wg.Wait()
	engine.Debug("a search waits")
	engine.Log(t.Context(), -8, "every step of a check")

	lines := read()
	if len(lines) != 8+8+1 {
		t.Errorf("the log has %d lines, want 17", len(lines))
	}
	has(t, find(lines, "a run started"), map[string]any{"level": "info", "from": "engine", "start": theLog.start, "searches": 3})
	has(t, find(lines, "a request failed"), map[string]any{"level": "error", "from": "api", "path": "/api/runs"})
	has(t, find(lines, "a search waits"), map[string]any{"level": "debug", "from": "engine"})
	if find(lines, "every step of a check") != nil {
		t.Error("a trace line was written to the file")
	}
	if !strings.Contains(string(theLog.recent.all()), "every step of a check") {
		t.Error("a trace line was not kept in memory")
	}
}

// A start of the app opens the log in its place and is a line of its own,
// with the build, so what follows is known to be from that build.
//
// Not parallel: it points the process's standard log at the app's log.
// On the real disk: lumberjack writes the log and knows only the disk.
func TestAStartSaysItsBuild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Cleanup(func() {
		theLog.close()
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	theLog.started()
	path := appLogPath()
	if !strings.HasPrefix(path, home) {
		t.Fatalf("the log is at %s, outside the home folder %s", path, home)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal(b, &line); err != nil {
		t.Fatalf("the first line is not one whole JSON object: %v\n%s", err, b)
	}
	has(t, line, map[string]any{"level": "info", "from": "app", "msg": "started", "build": runningVersion(), "start": theLog.start})
}

// The log keeps to its size: past 10 MB it starts a new file and keeps
// the one before beside it, so it never fills the disk. Lines written
// from many places at once while it does stay whole.
//
// On the real disk: lumberjack starts a new file of the log on the disk.
func TestTheLogKeepsToItsSize(t *testing.T) {
	path, read := testLog(t)
	text := strings.Repeat("y", 1000)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1500 {
				theLog.line(zerolog.DebugLevel, "app").Msg(text)
			}
		})
	}
	wg.Wait()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > appLogMB<<20 {
		t.Errorf("the log is %d bytes, more than %d MB", info.Size(), appLogMB)
	}
	older, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "app-*"))
	if len(older) == 0 {
		t.Error("no older file was kept beside the log")
	}
	read()
}

// The trace lines kept in memory are the last few thousand, oldest first,
// and lines written from many places at once stay whole.
func TestTheRecentTraceLinesAreTheLast(t *testing.T) {
	testLog(t)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range recentMost / 2 {
				theLog.line(zerolog.TraceLevel, "engine").Int("w", w).Int("i", i).Msg("a step")
			}
		})
	}
	wg.Wait()
	theLog.line(zerolog.TraceLevel, "engine").Msg("the last")
	kept := strings.Split(strings.TrimSuffix(string(theLog.recent.all()), "\n"), "\n")
	if len(kept) != recentMost {
		t.Errorf("%d lines are kept, want %d", len(kept), recentMost)
	}
	for _, line := range kept {
		if !json.Valid([]byte(line)) {
			t.Fatalf("a kept line is not whole: %s", line)
		}
	}
	if !strings.Contains(kept[len(kept)-1], "the last") {
		t.Errorf("the newest line is not last: %s", kept[len(kept)-1])
	}
}

// What a library or net/http says through the standard library's log
// goes into the app's log as a warning. It went to the standard error,
// which nobody reads on a Mac app opened from Finder.
//
// Not parallel: it points the process's standard log at the app's log.
// On the real disk: lumberjack writes the log and knows only the disk.
func TestTheStandardLogWritesIntoTheAppsLog(t *testing.T) {
	_, read := testLog(t)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	theLog.takeStandardLog()
	log.Printf("http: TLS handshake error from %s", "127.0.0.1")
	has(t, find(read(), "http: TLS handshake error"), map[string]any{
		"level": "warn", "from": "app", "msg": "http: TLS handshake error from 127.0.0.1",
	})
}

// A panic Wails catches goes into the app's log with its stack, and the
// app goes on. Wails' own handler ends the app.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestAPanicWailsCatchesIsInTheAppsLog(t *testing.T) {
	_, read := testLog(t)
	wailsPanicked(&application.PanicDetails{Error: errors.New("a window event nobody handled before"), FullStackTrace: "goroutine 1 [running]:"})
	has(t, find(read(), "stopped unexpectedly"), map[string]any{
		"level": "error", "from": "wails", "msg": "stopped unexpectedly: a window event nobody handled before", "stack": "goroutine 1 [running]:",
	})
}

// A panic in a request of the window fails that request, with its stack
// in the app's log, and the app goes on answering the others, several at
// once. Wails serves each request with no recover, so one ended the app.
func TestAPanicInARequestIsInTheAppsLog(t *testing.T) {
	_, read := testLog(t)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/people" {
			panic("a person nobody checked")
		}
		_, _ = w.Write([]byte(`{}`))
	})
	h := handler(nil, api)
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Go(func() {
			path := "/api/settings"
			if i%2 == 0 {
				path = "/api/people"
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			codes[i] = rec.Code
		})
	}
	wg.Wait()
	for i, code := range codes {
		want := http.StatusOK
		if i%2 == 0 {
			want = http.StatusInternalServerError
		}
		if code != want {
			t.Errorf("request %d answered %d, want %d", i, code, want)
		}
	}
	line := find(read(), "the window's request stopped unexpectedly")
	has(t, line, map[string]any{"level": "error", "from": "api", "path": "/api/people", "msg": "the window's request stopped unexpectedly: a person nobody checked"})
	if s, _ := line["stack"].(string); !strings.Contains(s, "goroutine") {
		t.Errorf("the line has no stack: %v", line)
	}

	// A handler that ends its response on purpose is no failure, and
	// net/http goes on to see it.
	abort := handler(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	func() {
		defer func() {
			if caught := recover(); caught != http.ErrAbortHandler {
				t.Errorf("an aborted response gave %v, want http.ErrAbortHandler again", caught)
			}
		}()
		abort.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	}()
}
