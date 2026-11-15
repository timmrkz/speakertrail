package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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
}

// The log is at most 10 MB of lines in use and three older files
// compressed beside it, a few MB more, whatever happens.
const (
	appLogMB    = 10
	appLogOlder = 3
)

var theLog = &appLog{start: startID()}

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
	return byLevel{file: l.file, recent: &l.recent}
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
}

func (s byLevel) Write(p []byte) (int, error) { return s.file.Write(p) }

func (s byLevel) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level == zerolog.TraceLevel {
		return s.recent.Write(p)
	}
	return s.file.Write(p)
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

// line starts a line of one part of the app at this moment, or gives nil,
// which takes fields and writes nothing, where its level is not written.
func (l *appLog) line(level zerolog.Level, from string) *zerolog.Event {
	return l.lineAt(level, from, time.Now())
}

// lineAt starts a line at a moment of its own.
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
// line goes to memory here too. FrameFairy's wrote straight to the file.
func (l *appLog) slog(from string, least zerolog.Level) *slog.Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return slog.New(slog.DiscardHandler)
	}
	lg := zerolog.New(l.writer()).Level(least).With().Str("start", l.start).Str("from", from).Logger()
	return slog.New(zerolog.NewSlogHandler(lg))
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
