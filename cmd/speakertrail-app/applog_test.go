package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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
		theLog.act.Store(nil)
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

// close stops writing the log, for the tests, the way the app's process
// ending does. Nothing watches the log then either.
func (l *appLog) close() {
	l.watching.Store(false)
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

// The window's lines reach the app's log as JSON, each under the moment
// the window said it, with its level and the start of the app, with what
// one request may write held to a few hundred lines of a few thousand
// characters. Several requests at once keep their lines whole. A trace
// line is kept in memory, not in the file.
func TestSaidWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)
	h := handler(nil, logAPI{log: theLog})
	say := func(lines []SaidLine) int {
		body, _ := json.Marshal(lines)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/log/said", bytes.NewReader(body)))
		return rec.Code
	}
	before := time.Now().Add(-3 * time.Second).Truncate(time.Millisecond)
	say([]SaidLine{
		{At: float64(before.UnixMilli()), Level: "error", Text: "uncaught: TypeError: x is undefined"},
		{Text: "window: hidden\n"},
		{Level: "trace", Text: "a step"},
		{At: float64(before.Add(-time.Hour).UnixMilli()), Level: "warn", Text: "a clock far off"},
	})
	var lots []SaidLine
	for i := range 430 {
		lots = append(lots, SaidLine{Text: fmt.Sprintf("line %d", i)})
	}
	say(lots)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() { say([]SaidLine{{Text: fmt.Sprintf("at once %d %s", i, strings.Repeat("x", 3000))}}) })
	}
	wg.Wait()
	if code := say(nil); code != http.StatusNoContent {
		t.Errorf("no lines answered %d, want 204", code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/log/said", strings.NewReader("[{")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("half a request answered %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/log/said", strings.NewReader(`[{"text":"`+strings.Repeat("y", saidMost)+`"}]`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a request past %d bytes answered %d, want 400", saidMost, rec.Code)
	}

	lines := read()
	uncaught := find(lines, "uncaught:")
	has(t, uncaught, map[string]any{"level": "error", "from": "window", "start": theLog.start})
	if at, _ := time.Parse(zerolog.TimeFieldFormat, fmt.Sprint(uncaught["time"])); !at.Equal(before) {
		t.Errorf("the line stands at %v, want the moment the window said it, %v", uncaught["time"], before)
	}
	if at, _ := time.Parse(zerolog.TimeFieldFormat, fmt.Sprint(find(lines, "a clock far off")["time"])); at.Before(before) {
		t.Errorf("a line from a clock an hour off stands at %v, want now", at)
	}
	has(t, find(lines, "window: hidden"), map[string]any{"level": "debug", "msg": "window: hidden"})
	if find(lines, "a step") != nil {
		t.Error("a trace line was written to the file")
	}
	if !strings.Contains(string(theLog.recent.all()), `"msg":"a step"`) {
		t.Errorf("the trace line is not kept in memory: %s", theLog.recent.all())
	}
	has(t, find(lines, "line 399"), map[string]any{"level": "debug"})
	has(t, find(lines, "and 30 lines more"), map[string]any{"level": "warn", "more": 30})
	if find(lines, "line 400") != nil {
		t.Error("one request wrote more than 400 lines")
	}
	for i := range 8 {
		start := fmt.Sprintf("at once %d ", i)
		if l := find(lines, start); l == nil || l["msg"] != start+strings.Repeat("x", 2000-len(start))+"…" {
			t.Errorf("the long line %d is not whole and cut at 2000 characters", i)
		}
	}
}

// What the person did is a line of its own, and every line it sets off
// carries its id: the window's, the Go side's, the API's and a run's,
// which keeps the act that started it however many acts follow while it
// goes.
func TestActsTieTheLinesTheySetOff(t *testing.T) {
	_, read := testLog(t)
	h := handler(nil, logAPI{log: theLog})
	act := func(a Act) {
		body, _ := json.Marshal(a)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/log/acted", bytes.NewReader(body)))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("an act answered %d", rec.Code)
		}
	}
	act(Act{ID: "a7", What: `pressed button "Run"`})
	api := theLog.slog("api", zerolog.TraceLevel)
	run := theLog.slog("engine", zerolog.TraceLevel).With("act", theLog.acting())
	theLog.said([]SaidLine{{Act: "a7", Text: "calls: POST /api/runs took 180 ms"}})
	api.Error("a request failed", "path", "/api/runs")
	theLog.line(zerolog.InfoLevel, "updates").Msg("channel picked")
	act(Act{ID: "a8", What: "Escape on nothing"})
	theLog.line(zerolog.DebugLevel, "window").Msg("the title bar was measured")
	run.Info("a run ended", "run", 3)
	api.Info("the next request")

	lines := read()
	has(t, find(lines, `pressed button "Run"`), map[string]any{"level": "info", "from": "window", "act": "a7"})
	has(t, find(lines, "calls: POST"), map[string]any{"act": "a7"})
	has(t, find(lines, "a request failed"), map[string]any{"act": "a7", "from": "api"})
	has(t, find(lines, "channel picked"), map[string]any{"act": "a7", "from": "updates"})
	has(t, find(lines, "the title bar was measured"), map[string]any{"act": "a8"})
	has(t, find(lines, "a run ended"), map[string]any{"act": "a7", "from": "engine", "run": 3})
	has(t, find(lines, "the next request"), map[string]any{"act": "a8"})
	if n := strings.Count(fmt.Sprint(find(lines, "a run ended")), "act:"); n != 1 {
		t.Errorf("a run's line names %d acts, want 1", n)
	}
}

// Settings shows the app's log, its files with the older ones, their size
// and their lines, shows it in its folder, and removes them all, and
// nothing else in their folder. The next line starts a new log, which
// says what went.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestTheLogCanBeRemovedWhole(t *testing.T) {
	path, read := testLog(t)
	dir := filepath.Dir(path)
	for i := range 4 {
		theLog.line(zerolog.InfoLevel, "app").Int("i", i).Msg("a line before")
	}
	var older bytes.Buffer
	z := gzip.NewWriter(&older)
	_, _ = z.Write([]byte("{}\n{}\n{}\n"))
	_ = z.Close()
	if err := os.WriteFile(filepath.Join(dir, "app-2026-10-09T10-00-00.000.log.gz"), older.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	var shown []string
	h := handler(nil, logAPI{log: theLog, show: func(p string) error { shown = append(shown, p); return nil }})
	ask := func(method, path string) (int, LogStatus) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		var st LogStatus
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		return rec.Code, st
	}
	if _, st := ask(http.MethodGet, "/api/log"); st.Path != path || st.Files != 2 || st.Lines != 4+3 || st.Bytes <= int64(older.Len()) {
		t.Fatalf("the log is %+v, want %s in 2 files with 7 lines", st, path)
	}
	if code, _ := ask(http.MethodPost, "/api/log/show"); code != http.StatusNoContent || len(shown) != 1 || shown[0] != path {
		t.Fatalf("show answered %d and showed %v, want %s", code, shown, path)
	}

	// Lines keep coming while it is removed, from everywhere at once.
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				theLog.line(zerolog.DebugLevel, "app").Msg("a line while it goes")
			}
		})
	}
	code, after := ask(http.MethodDelete, "/api/log")
	wg.Wait()
	if code != http.StatusOK {
		t.Fatalf("remove answered %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-2026-10-09T10-00-00.000.log.gz")); !os.IsNotExist(err) {
		t.Fatalf("the older file of the log is still there, %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "notes.txt")); err != nil || string(b) != "mine" {
		t.Fatalf("a file beside the log that is not the log's went too, %v", err)
	}
	if after.Files != 1 || after.Path != path {
		t.Fatalf("after removing, the log is %+v, want one new file", after)
	}
	lines := read()
	if find(lines, "a line before") != nil {
		t.Fatal("a line from before is still in the log")
	}
	gone := find(lines, "the app's log was removed")
	// The lines said while it went count too, so at least the seven.
	if n, _ := gone["lines"].(float64); gone == nil || gone["files"] != float64(2) || n < 7 {
		t.Fatalf("the new log says %v, want what went", gone)
	}
}

// An older file of the log is packed and never changes, so Settings
// unpacks it once and not again on every look. Its packed size is far
// smaller than the lines in it, which once had it read again each time,
// about 0.1 s a look with a full log.
func TestAnOlderFileIsReadOnce(t *testing.T) {
	path, _ := testLog(t)
	theLog.line(zerolog.InfoLevel, "app").Msg("a line in use")
	var older bytes.Buffer
	z := gzip.NewWriter(&older)
	_, _ = z.Write(bytes.Repeat([]byte(`{"level":"debug","msg":"an older line"}`+"\n"), 1000))
	_ = z.Close()
	packed := filepath.Join(filepath.Dir(path), "app-2026-10-09T10-00-00.000.log.gz")
	if err := os.WriteFile(packed, older.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := theLog.status(); st.Files != 2 || st.Lines != 1001 {
		t.Fatalf("the log is %d files with %d lines, want 2 with 1001", st.Files, st.Lines)
	}
	// Changed in place, the same file, it would read as no lines at all
	// if it were read again.
	f, err := os.OpenFile(packed, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("not a packed file")
	_ = f.Close()
	theLog.line(zerolog.InfoLevel, "app").Msg("another line in use")
	if st := theLog.status(); st.Lines != 1002 {
		t.Fatalf("the log has %d lines, want 1002: the older file was read again", st.Lines)
	}
}

// Settings shows the last five toasts about something that went wrong,
// newest first, read back from the log, the older files too, with when
// each came. A toast said again stands once, where it was said last. A
// line that only names a toast, one from elsewhere and one cut off are no
// toasts. Removing the log takes them away too.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestSettingsShowsTheLastToasts(t *testing.T) {
	path, _ := testLog(t)
	dir := filepath.Dir(path)
	if st := theLog.status(); len(st.Toasts) != 0 {
		t.Fatalf("a log with no toast shows %v", st.Toasts)
	}

	// Two toasts in the older file, from before the log turned over.
	var older bytes.Buffer
	z := gzip.NewWriter(&older)
	_, _ = z.Write([]byte(`{"level":"warn","time":"2026-11-12T09:00:00.000+01:00","start":"a","from":"window","toast":true,"msg":"one"}` + "\n" +
		`{"level":"warn","time":"2026-11-12T09:01:00.000+01:00","start":"a","from":"window","toast":true,"msg":"two"}` + "\n"))
	_ = z.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app-2026-11-12T09-02-00.000.log.gz"), older.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-30 * time.Second).Truncate(time.Millisecond)
	at := func(i int) float64 { return float64(start.Add(time.Duration(i) * time.Second).UnixMilli()) }
	theLog.said([]SaidLine{
		{At: at(0), Level: "warn", Toast: true, Text: "three"},
		{At: at(1), Level: "info", Text: "toast: Saved the Exa key in the keychain"},
		{At: at(2), Level: "warn", Toast: true, Text: "Could not reach the app. Try again."},
		{At: at(3), Level: "warn", Toast: true, Text: "four"},
		{At: at(4), Level: "warn", Text: "the window said \"toast\":true as words"},
		{At: at(5), Level: "warn", Toast: true, Text: "five"},
		{At: at(6), Level: "warn", Toast: true, Text: "four"},
	})
	theLog.line(zerolog.WarnLevel, "app").Bool("toast", true).Msg("not the window's")
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		t.Fatal(err)
	} else {
		_, _ = f.WriteString(`{"level":"warn","from":"window","toast":true,"msg":"cut of` + "\n")
		_ = f.Close()
	}

	got := theLog.status().Toasts
	want := []LoggedToast{
		{At: int64(at(6)), Text: "four"},
		{At: int64(at(5)), Text: "five"},
		{At: int64(at(2)), Text: "Could not reach the app. Try again."},
		{At: int64(at(0)), Text: "three"},
		{At: time.Date(2026, 11, 12, 8, 1, 0, 0, time.UTC).UnixMilli(), Text: "two"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Settings shows the toasts\n%v\nwant the last five, newest first\n%v", got, want)
	}

	if _, err := theLog.removed(); err != nil {
		t.Fatal(err)
	}
	if st := theLog.status(); len(st.Toasts) != 0 {
		t.Fatalf("the log was removed, and Settings still shows %v", st.Toasts)
	}
}

// Settings, while it is open, is sent the log as lines are written,
// without a reload: the toasts that came and the lines counted, once a
// beat however many lines come in it, the same as read afresh. A log
// turned over is a new file of the same name, and is counted as one.
// The lines Wails and the engine write through slog wake it too.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestSettingsSeesTheLogAsItIsWritten(t *testing.T) {
	l := &appLog{start: "watch", grew: make(chan struct{}, 1)}
	l.open(filepath.Join(t.TempDir(), "logs", "app.log"))
	t.Cleanup(l.close)
	// Turned over by hand below, and kept plain, since packing it runs
	// on lumberjack's own goroutine.
	l.file.Compress = false
	told := make(chan LogStatus, 4)
	beat := make(chan time.Time)
	done := make(chan struct{})
	toast := func(text string) { l.line(zerolog.WarnLevel, "window").Bool("toast", true).Msg(text) }
	shows := func(st LogStatus, files, lines int, texts ...string) {
		t.Helper()
		got := []string{}
		for _, t := range st.Toasts {
			got = append(got, t.Text)
		}
		if st.Files != files || st.Lines != lines || !slices.Equal(got, texts) {
			t.Fatalf("Settings shows %d files, %d lines and %v, want %d, %d and %v", st.Files, st.Lines, got, files, lines, texts)
		}
	}

	// Settings opens on a log already written, and is told of it once.
	// Watched from here, so the line before it and the opening leave one
	// wake between them. A watch already waiting may take the line's wake
	// and then the opening's, which tells Settings twice, a beat apart.
	// That is harmless in the app, and a test that counts messages would
	// not know which it got.
	toast("before")
	l.watchFor(true)
	go func() {
		l.watch(func(st LogStatus) { told <- st }, func() <-chan time.Time { return beat })
		close(done)
	}()
	shows(<-told, 1, 1, "before")
	beat <- time.Time{}
	toast("one")
	shows(<-told, 1, 2, "one", "before")

	// Two lines in one beat are one message, sent once the beat is over.
	// One of them is Wails's, through slog.
	l.slog("wails", zerolog.InfoLevel).Info("a line of Wails")
	toast("two")
	beat <- time.Time{}
	shows(<-told, 1, 4, "two", "one", "before")

	// The same as the log read afresh.
	l.counting.Lock()
	l.counted = nil
	l.counting.Unlock()
	shows(l.status(), 1, 4, "two", "one", "before")

	// Turned over, the old file keeps its lines under a new name, and a
	// new one starts under the old name. It grows past where the old one
	// was read, and is still read from its start.
	if err := l.file.Rotate(); err != nil {
		t.Fatal(err)
	}
	for range 6 {
		l.line(zerolog.InfoLevel, "app").Msg("a line in the new file")
	}
	toast("three")
	beat <- time.Time{}
	shows(<-told, 2, 11, "three", "two", "one", "before")

	// Settings closed, nothing more is sent.
	l.watchFor(false)
	beat <- time.Time{}
	toast("four")
	close(l.grew)
	<-done
	select {
	case st := <-told:
		t.Fatalf("Settings was closed and was sent %+v", st)
	default:
	}
}
