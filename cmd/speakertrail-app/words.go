package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Small helpers for what Report a Problem and the speech models say,
// from FrameFairy's bugs.go and report.go.

// cut makes text valid UTF-8 and cuts it to at most max bytes, between two
// letters, never in the middle of one: a German report cut in half an ä
// would end in a byte no reader can show.
func cut(text string, max int) string {
	text = strings.ToValidUTF8(text, "")
	if len(text) <= max {
		return text
	}
	for max > 0 && !utf8.RuneStart(text[max]) {
		max--
	}
	return text[:max]
}

// sentence starts a reason with a capital letter.
func sentence(s string) string {
	if s == "" {
		return ""
	}
	first, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(first)) + s[size:]
}

// tidyName is a file name fit for the log and a tile's title: no folder,
// nothing that is not printable, not too long.
func tidyName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = "picture"
	}
	return cut(name, 120)
}

// homeless writes the home folder as ~ wherever it is named, so the Mac's
// user name does not travel in a report.
func homeless(b []byte) []byte {
	home, err := os.UserHomeDir()
	if err != nil || len(home) < 2 {
		return b
	}
	return bytes.ReplaceAll(b, []byte(home), []byte("~"))
}

// tildePath is a path as the window shows it, the home folder as ~.
func tildePath(path string) string {
	return string(homeless([]byte(path)))
}
