package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
	"gopkg.in/natefinch/lumberjack.v2"
)

// The app's own log: what the app did, what a run did and what went
// wrong, so a fault on Tim's Mac is found by reading what happened, not by
// asking him to try again. It is in the place Mac apps keep theirs, which
// the Console app shows: ~/Library/Logs/Speaker Trail/app.log. Elsewhere
// it is in the user's cache folder. Ported from FrameFairy's applog.go,
// see docs/LOGGING.md.
//
// zerolog writes the lines, one JSON object each, and lumberjack keeps the
// file to its size. Nothing here does either job itself.
//
//	{"level":"info","time":"2026-11-14T18:41:07.512+01:00","start":"9f3a01","from":"updates","channel":"main","msg":"channel picked, its newest build is looked for"}
//
// Every line from debug up is always written, so the steps that led to a
// fault are there when it happens. What keeps the disk free is the cap
// below, not a switch. The trace lines, which come with every step of
// something that repeats, are kept in memory instead, the last few
// thousand of them, for the report of a problem. See recent.
type appLog struct {
	mu   sync.Mutex
	file *lumberjack.Logger
	// The logger lines are written through, nil while the log is not open,
	// which is so in the tests that do not open one.
	root atomic.Pointer[zerolog.Logger]
	// One start of the app, so a restart is seen and two starts never mix.
	// FrameFairy calls it run, which here is a run of searches.
	start  string
	recent recentLines
	// The person's last act, which the Go side's lines carry, see Acted.
	act atomic.Pointer[string]
	// counted is what status read of each file, so the next look reads
	// only what was written since, see count.
	counting sync.Mutex
	counted  map[string]fileCount
	// grew hears that a line went into the file, and watching whether
	// Settings is open to be told, see watch.
	grew     chan struct{}
	watching atomic.Bool
}

// The log is at most 10 MB of lines in use and three older files
// compressed beside it, a few MB more, whatever happens.
const (
	appLogMB    = 10
	appLogOlder = 3
)

var theLog = &appLog{start: startID(), grew: make(chan struct{}, 1)}

func init() {
	// A moment to the millisecond, with the Mac's offset from UTC.
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z07:00"
	// msg, as log/slog and most tools that read logs call it.
	zerolog.MessageFieldName = "msg"
	// zerolog holds every logger to debug and up unless told otherwise.
	// Which lines are written is the logger's own level.
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
}

func startID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func appLogPath() string {
	home, err := os.UserHomeDir()
	if runtime.GOOS == "darwin" && err == nil {
		return filepath.Join(home, "Library", "Logs", "Speaker Trail", "app.log")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "SpeakerTrail", "logs", "app.log")
}

// open starts writing the log at path. Lines said before it go nowhere.
func (l *appLog) open(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
	}
	l.file = &lumberjack.Logger{Filename: path, MaxSize: appLogMB, MaxBackups: appLogOlder, Compress: true}
	lg := zerolog.New(l.writer()).Level(zerolog.TraceLevel)
	l.root.Store(&lg)
}

// writer is where every logger of the log writes, the trace lines to
// memory and the rest to the file. Called with mu held.
func (l *appLog) writer() byLevel {
	return byLevel{file: l.file, recent: &l.recent, grew: l.grew}
}

// wailsPanicked writes a panic Wails caught into the app's log, with its
// stack, see the PanicHandler in main.
func wailsPanicked(d *application.PanicDetails) {
	theLog.line(zerolog.ErrorLevel, "wails").Str("stack", d.FullStackTrace).Msgf("stopped unexpectedly: %v", d.Error)
}

// byLevel sends the trace lines to memory and every other line to the file.
// zerolog hands a writer that can tell levels apart each line with its
// level.
type byLevel struct {
	file   io.Writer
	recent *recentLines
	// grew is told that the file has a line more, without waiting for
	// anyone to hear it, see watch.
	grew chan struct{}
}

func (s byLevel) Write(p []byte) (int, error) { return s.toFile(p) }

func (s byLevel) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level == zerolog.TraceLevel {
		return s.recent.Write(p)
	}
	return s.toFile(p)
}

func (s byLevel) toFile(p []byte) (int, error) {
	n, err := s.file.Write(p)
	select {
	case s.grew <- struct{}{}:
	default:
	}
	return n, err
}

// The trace lines kept, a few MB of memory at most.
const recentMost = 8000

// recentLines are the last trace lines, the oldest given way to the
// newest.
type recentLines struct {
	mu    sync.Mutex
	lines [][]byte
	next  int
}

func (r *recentLines) Write(p []byte) (int, error) {
	line := append([]byte(nil), p...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) < recentMost {
		r.lines = append(r.lines, line)
	} else {
		r.lines[r.next] = line
		r.next = (r.next + 1) % recentMost
	}
	return len(p), nil
}

// all are the lines kept, oldest first.
func (r *recentLines) all() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []byte
	for i := range r.lines {
		out = append(out, r.lines[(r.next+i)%len(r.lines)]...)
	}
	return out
}

// line starts a line of one part of the app at this moment, with the act
// it belongs to, or gives nil, which takes fields and writes nothing,
// where its level is not written.
func (l *appLog) line(level zerolog.Level, from string) *zerolog.Event {
	return withAct(l.lineAt(level, from, time.Now()), l.acting())
}

func withAct(ev *zerolog.Event, act string) *zerolog.Event {
	if act == "" {
		return ev
	}
	return ev.Str("act", act)
}

// acting is the person's last act, or empty before the first.
func (l *appLog) acting() string {
	if a := l.act.Load(); a != nil {
		return *a
	}
	return ""
}

// lineAt starts a line at a moment of its own, with no act.
func (l *appLog) lineAt(level zerolog.Level, from string, at time.Time) *zerolog.Event {
	lg := l.root.Load()
	if lg == nil {
		return nil
	}
	return lg.WithLevel(level).Time(zerolog.TimestampFieldName, at).Str("start", l.start).Str("from", from)
}

// slog is a logger for code that speaks log/slog, whose lines go into the
// same log as the part named, from the level given: Wails, which runs the
// app's window, and the engine and the API. Lines said before the log is
// open go nowhere.
//
// Its lines go through the same writer as every other line, so a trace
// line goes to memory here too and Settings hears of it, and they carry
// the person's last act, like every line of ours. FrameFairy's wrote
// straight to the file, with no act.
func (l *appLog) slog(from string, least zerolog.Level) *slog.Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return slog.New(slog.DiscardHandler)
	}
	lg := zerolog.New(l.writer()).Level(least).With().Str("start", l.start).Str("from", from).Logger()
	return slog.New(actHandler{Handler: zerolog.NewSlogHandler(lg), log: l})
}

// actHandler adds the person's last act to a line that names none. A run
// names the act that started it, see engine.Engine.Act.
type actHandler struct {
	slog.Handler
	log *appLog
	// named says the logger names an act of its own.
	named bool
}

func (h actHandler) Handle(ctx context.Context, r slog.Record) error {
	if !h.named {
		named := false
		r.Attrs(func(a slog.Attr) bool {
			named = a.Key == "act"
			return !named
		})
		if act := h.log.acting(); act != "" && !named {
			r = r.Clone()
			r.AddAttrs(slog.String("act", act))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h actHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	named := h.named
	for _, a := range attrs {
		named = named || a.Key == "act"
	}
	return actHandler{Handler: h.Handler.WithAttrs(attrs), log: h.log, named: named}
}

func (h actHandler) WithGroup(name string) slog.Handler {
	return actHandler{Handler: h.Handler.WithGroup(name), log: h.log, named: h.named}
}

// started opens the log in its place and notes a start of the app, with
// its build, so what follows is known to be from that build.
func (l *appLog) started() {
	l.open(appLogPath())
	l.takeStandardLog()
	l.line(zerolog.InfoLevel, "app").Str("build", runningVersion()).Str("channel", buildChannel).Str("commit", buildCommit).
		Str("os", runtime.GOOS).Str("arch", runtime.GOARCH).Msg("started")
}

// takeStandardLog writes what is said through the standard library's log,
// by a library or by net/http, into the app's log as a warning, and where
// it went before. Only the standard error had it, which nobody reads on a
// Mac app opened from Finder. Our own code logs through theLog, see the
// lint in .golangci.yml.
func (l *appLog) takeStandardLog() {
	// The app's log has a field for the moment.
	log.SetFlags(0)
	log.SetOutput(io.MultiWriter(log.Writer(), standardLines{l}))
}

// standardLines is the standard library's log, a line at a time.
type standardLines struct{ l *appLog }

func (s standardLines) Write(p []byte) (int, error) {
	s.l.line(zerolog.WarnLevel, "app").Msg(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// files are the log's files, the older ones first, the one in use last,
// or none while it is not open.
func (l *appLog) files() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	path := l.file.Filename
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	older, _ := filepath.Glob(filepath.Join(filepath.Dir(path), base+"-*"))
	sort.Strings(older)
	return append(older, path)
}

// LogStatus is the app's log as Settings shows it: where the file in use
// is, how many files there are with the older ones, and how much they hold
// together.
type LogStatus struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
	Lines int    `json:"lines"`
	// Toasts are the last toasts the app showed about something that went
	// wrong, newest first, at most toastsShown of them.
	Toasts []LoggedToast `json:"toasts"`
}

// LoggedToast is a toast the app showed, read back from its log: when, in
// ms since 1970, and its words. Settings shows the last few, so a toast
// that ran out unread is not lost.
type LoggedToast struct {
	At   int64  `json:"at"`
	Text string `json:"text"`
}

// toastsShown is how many toasts Settings shows, the last five, as in
// FrameFairy.
const toastsShown = 5

// toastsIn adds the toasts the lines of data say to toasts, in the order
// they were said: the lines the window marked as a toast, see Said. A
// toast said again takes the place of the one before, so one failure
// that keeps coming back does not push every other toast out of the last
// five. A line that is not whole JSON is passed over, the way a line cut
// off by a crash is.
func toastsIn(data []byte, toasts []LoggedToast) []LoggedToast {
	for line := range bytes.Lines(data) {
		if !bytes.Contains(line, []byte(`"toast":true`)) {
			continue
		}
		var l struct {
			Time  string `json:"time"`
			From  string `json:"from"`
			Toast bool   `json:"toast"`
			Msg   string `json:"msg"`
		}
		if json.Unmarshal(line, &l) != nil || !l.Toast || l.From != "window" || l.Msg == "" {
			continue
		}
		at, _ := time.Parse(zerolog.TimeFieldFormat, l.Time)
		toasts = laterToast(toasts, LoggedToast{At: at.UnixMilli(), Text: l.Msg})
	}
	return toasts
}

// laterToast is toasts with t said last, in the place of the same toast
// said before.
func laterToast(toasts []LoggedToast, t LoggedToast) []LoggedToast {
	toasts = slices.DeleteFunc(toasts, func(o LoggedToast) bool { return o.Text == t.Text })
	return append(toasts, t)
}

// fileCount is what one file of the log held when it was last read: which
// file it was, how far it was read, the lines up to there and the toasts
// in them.
type fileCount struct {
	file   os.FileInfo
	read   int64
	lines  int
	toasts []LoggedToast
}

// count is what the file at path holds now, from what it held when was
// was read. An older file never changes, so once it was read whole it is
// not read again. The file in use only grows until it is turned over, so
// only what was written since is read, up to its last whole line, and a
// line being written is read the next time. A file of the same name that
// is not the same file is a new one, the log turned over or removed, and
// is read whole.
//
// An older file is packed, so its size on the disk says nothing of the
// bytes read from it, which are unpacked. FrameFairy's count compares the
// two, and so reads every older file again on every look.
func count(path string, info os.FileInfo, was fileCount) fileCount {
	packed := strings.HasSuffix(path, ".gz")
	if was.file == nil || !os.SameFile(was.file, info) || !packed && info.Size() < was.read {
		was = fileCount{}
	}
	was.file = info
	size := info.Size()
	if was.read > 0 && (size == was.read || packed) {
		return was
	}
	var data []byte
	if packed {
		all, err := readLog(path)
		if err != nil {
			return was
		}
		data = all
	} else {
		f, err := os.Open(path)
		if err != nil {
			return was
		}
		defer f.Close()
		if _, err := f.Seek(was.read, io.SeekStart); err != nil {
			return was
		}
		if data, err = io.ReadAll(io.LimitReader(f, size-was.read)); err != nil {
			return was
		}
		data = data[:bytes.LastIndexByte(data, '\n')+1]
	}
	return fileCount{
		file:   info,
		read:   was.read + int64(len(data)),
		lines:  was.lines + bytes.Count(data, []byte("\n")),
		toasts: toastsIn(data, slices.Clone(was.toasts)),
	}
}

// readLog is a file of the log, unpacked when it is an older one.
func readLog(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(path, ".gz") {
		return data, err
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 64<<20))
}

// status counts the log's files, their bytes on the disk and their lines,
// the older ones unpacked to count them, and the last toasts in them.
// What was counted before is not counted again, see count, so Settings
// can be kept up to date while lines are written.
func (l *appLog) status() LogStatus {
	files := l.files()
	if len(files) == 0 {
		return LogStatus{Toasts: []LoggedToast{}}
	}
	st := LogStatus{Path: files[len(files)-1]}
	toasts := []LoggedToast{}
	l.counting.Lock()
	defer l.counting.Unlock()
	counted := make(map[string]fileCount, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		st.Files++
		st.Bytes += info.Size()
		now := count(path, info, l.counted[path])
		counted[path] = now
		st.Lines += now.lines
		for _, t := range now.toasts {
			toasts = laterToast(toasts, t)
		}
	}
	// A file gone is forgotten.
	l.counted = counted
	toasts = toasts[max(0, len(toasts)-toastsShown):]
	slices.Reverse(toasts)
	st.Toasts = toasts
	return st
}

// watch tells tell the log as Settings shows it while Settings watches,
// after lines were written, and then waits for beat before it tells
// again, so a busy log is told about once a beat and not once a line. It
// runs until grew is closed, which only a test does.
func (l *appLog) watch(tell func(LogStatus), beat func() <-chan time.Time) {
	for range l.grew {
		if !l.watching.Load() {
			continue
		}
		tell(l.status())
		<-beat()
	}
}

// watchFor says whether Settings is open, so the log it shows is sent to
// it as lines are written, as the event applog.
func (l *appLog) watchFor(on bool) {
	l.watching.Store(on)
	if on {
		select {
		case l.grew <- struct{}{}:
		default:
		}
	}
}

// remove removes every file of the log, the older ones too, and the next
// line starts a new one. Only the log's own files go, the ones files
// gives, never anything else in its folder.
func (l *appLog) remove() (LogStatus, error) {
	was := l.status()
	files := l.files()
	l.mu.Lock()
	var failed error
	for _, path := range files {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && failed == nil {
			failed = err
		}
	}
	// Closed only once its file is gone, and lumberjack opens a new one
	// with the next line. Closed first, a line said in between opened the
	// file again, which was then removed under it, and every line after
	// went into a file that was no longer there.
	if l.file != nil {
		_ = l.file.Close()
	}
	l.mu.Unlock()
	l.counting.Lock()
	l.counted = nil
	l.counting.Unlock()
	return was, failed
}

// removed removes every file of the app's log, after Settings asked
// first, and gives the log as it is then: the one line that says so.
func (l *appLog) removed() (LogStatus, error) {
	was, err := l.remove()
	line := l.line(zerolog.InfoLevel, "app").Int("files", was.Files).Int64("bytes", was.Bytes).Int("lines", was.Lines)
	if err != nil {
		line.Err(err).Msg("the app's log could not be removed whole")
		return l.status(), fmt.Errorf("the log could not be removed whole: %w", err)
	}
	line.Msg("the app's log was removed")
	return l.status(), nil
}

// What the window may write at once: a few hundred lines of a few hundred
// characters. The window is our own code, but a loop gone wrong in it
// should not fill the disk.
const (
	saidLines   = 400
	saidLineMax = 2000
)

// SaidLine is one line the window says, with the moment it said it, in ms
// since 1970, so the window's lines and the Go side's stand in the order
// they happened, though the window sends its own a moment later. Its
// level is error, warn, info, debug or trace, debug when it says none.
// Toast marks a toast the app showed about something that went wrong,
// which Settings reads back, see toastsIn.
type SaidLine struct {
	At    float64 `json:"at"`
	Level string  `json:"level,omitempty"`
	Act   string  `json:"act,omitempty"`
	Toast bool    `json:"toast,omitempty"`
	Text  string  `json:"text"`
}

// levelOf is the level the window named.
func levelOf(name string) zerolog.Level {
	switch name {
	case "error":
		return zerolog.ErrorLevel
	case "warn":
		return zerolog.WarnLevel
	case "info":
		return zerolog.InfoLevel
	case "trace":
		return zerolog.TraceLevel
	}
	return zerolog.DebugLevel
}

// said writes what the window reports into the app's log: its errors and
// warnings, its calls that failed or were slow, and its acts slow to reach
// the screen, see web/src/lib/said.ts.
func (l *appLog) said(lines []SaidLine) {
	more := 0
	if len(lines) > saidLines {
		more = len(lines) - saidLines
		lines = lines[:saidLines]
	}
	now := time.Now()
	for _, line := range lines {
		text := strings.TrimRight(line.Text, "\n")
		if len(text) > saidLineMax {
			text = text[:saidLineMax] + "…"
		}
		at := now
		// A moment from the window that is far off is the window's clock,
		// not when it was said.
		if t := time.UnixMilli(int64(line.At)); line.At > 0 && t.Sub(now).Abs() < time.Minute {
			at = t
		}
		ev := l.lineAt(levelOf(line.Level), "window", at)
		if line.Toast {
			ev = ev.Bool("toast", true)
		}
		withAct(ev, clipped(line.Act, actMax)).Msg(text)
	}
	if more > 0 {
		l.line(zerolog.WarnLevel, "window").Int("more", more).Msgf("and %d lines more", more)
	}
}

// An act's id and what it says are the window's own few words.
const (
	actMax     = 16
	actWhatMax = 300
)

func clipped(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Act is one thing the person did, a click or a key, with the id every
// line it sets off carries, the moment, in ms since 1970, and a few words
// on what it was. See web/src/lib/said.ts.
type Act struct {
	ID   string  `json:"id"`
	At   float64 `json:"at"`
	What string  `json:"what"`
}

// acted notes what the person did, at once rather than with the window's
// next lines, so that what the Go side does for it carries its id: the
// run it starts, the request it makes. The window says each act before
// the requests it makes for it.
func (l *appLog) acted(a Act) {
	id := clipped(a.ID, actMax)
	l.act.Store(&id)
	at := time.Now()
	if t := time.UnixMilli(int64(a.At)); a.At > 0 && t.Sub(at).Abs() < time.Minute {
		at = t
	}
	withAct(l.lineAt(zerolog.InfoLevel, "window", at), id).Msg(clipped(a.What, actWhatMax))
}
