package main

import (
	"context"
	"log/slog"
)

// theLog writes a line of the app's log in the shape the update code,
// ported from FrameFairy, writes it: a level and a part, then the fields,
// then the message. It goes to slog, which is the app's log.
var theLog appLog

type appLog struct{}

// logLine is one line being written.
type logLine struct {
	level slog.Level
	attrs []any
}

func (appLog) line(level slog.Level, part string) *logLine {
	return &logLine{level: level, attrs: []any{"part", part}}
}

func (l *logLine) Str(key, value string) *logLine {
	l.attrs = append(l.attrs, key, value)
	return l
}

func (l *logLine) Float64(key string, value float64) *logLine {
	l.attrs = append(l.attrs, key, value)
	return l
}

func (l *logLine) Bool(key string, value bool) *logLine {
	l.attrs = append(l.attrs, key, value)
	return l
}

func (l *logLine) Interface(key string, value any) *logLine {
	l.attrs = append(l.attrs, key, value)
	return l
}

func (l *logLine) Err(err error) *logLine {
	if err != nil {
		l.attrs = append(l.attrs, "error", err.Error())
	}
	return l
}

func (l *logLine) Msg(msg string) {
	slog.Default().Log(context.Background(), l.level, msg, l.attrs...)
}
