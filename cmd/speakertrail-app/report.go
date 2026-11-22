package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/timmrkz/speakertrail/internal/speech/asr"
	"github.com/timmrkz/speakertrail/internal/store"
)

// A report of a problem: one zip on the Desktop, with what Tim says
// happened, the app's log and what is needed to read it. It never holds
// the app's file or the names of the people in it, and paths under the
// home folder are written as ~, so the Mac's user name does not travel.
// With People switched on it holds people.json, every person without
// name, link, notes or first message, see store.Shared.
// Bugs in the sidebar, bugs.go and docs/LOGGING.md. Ported from
// FrameFairy's report.go.

// What Tim may write, a few pages at most.
const reportWordsMax = 20000

// reportFolder is where a report is put: the Desktop, or the home folder
// when there is none.
func reportFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	if desk := filepath.Join(home, "Desktop"); dirExists(desk) {
		return desk
	}
	return home
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// aboutMachine is what a report says about the app and the Mac.
type aboutMachine struct {
	Made      string `json:"made"`
	Build     string `json:"build"`
	Start     string `json:"start"`
	OS        string `json:"os"`
	OSVersion string `json:"osVersion,omitempty"`
	Arch      string `json:"arch"`
	Chip      string `json:"chip,omitempty"`
	MemoryGB  int64  `json:"memoryGB,omitempty"`
}

// asked runs a short command for a report and gives what it said, or
// nothing.
func asked(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func machine() aboutMachine {
	a := aboutMachine{
		Made: time.Now().Format(time.RFC3339), Build: runningVersion(), Start: theLog.start,
		OS: runtime.GOOS, Arch: runtime.GOARCH, MemoryGB: asr.ThisMachine().Memory >> 30,
	}
	if runtime.GOOS == "darwin" {
		a.OSVersion = asked("sw_vers", "-productVersion")
		a.Chip = asked("sysctl", "-n", "machdep.cpu.brand_string")
	}
	return a
}

// reportMaker makes a report from the draft, with the settings of the
// app's file and nothing else of it.
type reportMaker struct {
	report *reporting
	st     *store.Store
	// folder is where a report is put, the Desktop when it is empty, and
	// show how it is shown, in Finder when it is empty. A test hands in
	// its own.
	folder func() string
	show   func(path string) error
}

// ReportProblem writes a report of a problem to the Desktop, shows it in
// Finder and closes the report window, with what Tim says happened, the
// pictures he attached and People when he switched them on. It gives the
// report's path, and lets go of the draft it was made from.
func (m reportMaker) ReportProblem(what string, people bool) (string, error) {
	what = cut(what, reportWordsMax)
	sent, pictures := m.report.reportPictures()
	theLog.line(zerolog.InfoLevel, "report").Int("characters", len(what)).Int("pictures", len(pictures)).Bool("people", people).Msg("a report of a problem is being made")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := z.Create("Speaker Trail report/" + name)
		if err != nil {
			return err
		}
		_, err = w.Write(homeless(data))
		return err
	}
	if strings.TrimSpace(what) != "" {
		if err := add("what-happened.txt", []byte(strings.TrimSpace(what)+"\n")); err != nil {
			return "", err
		}
	}
	about, _ := json.MarshalIndent(machine(), "", "  ")
	if err := add("about.json", about); err != nil {
		return "", err
	}
	// The settings Settings shows, their keys, values and what they are
	// for. Never the people, the searches or the notes, and never a key,
	// which lives in the keychain.
	// The first message template as Settings has it, written the way
	// internal/store/first_message_template.json is, so Tim's changes
	// can be taken into the code as they are.
	if m.st != nil {
		var settings []store.Setting
		var template store.Template
		m.st.Read(func(d *store.Data) { settings, template = d.AllSettings(), d.FirstMessageTemplate() })
		body, _ := json.MarshalIndent(settings, "", "  ")
		if err := add("settings.json", body); err != nil {
			return "", err
		}
		body, err := json.MarshalIndent(template, "", "  ")
		if err != nil {
			return "", err
		}
		if err := add("first-message-template.json", append(body, '\n')); err != nil {
			return "", err
		}
		// People, as Tim chose on 22 November 2026, issue #73: what the
		// person screen shows and the profile text, never a name or a
		// link.
		if people {
			var shared []store.Shared
			m.st.Read(func(d *store.Data) { shared = d.Share() })
			body, err := json.MarshalIndent(shared, "", "  ")
			if err != nil {
				return "", err
			}
			if err := add("people.json", append(body, '\n')); err != nil {
				return "", err
			}
			theLog.line(zerolog.InfoLevel, "report").Int("people", len(shared)).Msg("the report takes People along")
		}
	}
	for _, path := range theLog.files() {
		data, err := readLog(path)
		if err != nil {
			continue
		}
		if err := add("logs/"+logName(path), data); err != nil {
			return "", err
		}
	}
	// The pictures go as they are, since home is not in a picture's
	// pixels and replacing it in them would spoil the file.
	for i, data := range pictures {
		w, err := z.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("Speaker Trail report/pictures/%d.png", i+1), Method: zip.Deflate})
		if err == nil {
			_, err = w.Write(data)
		}
		if err != nil {
			return "", err
		}
	}
	// The trace lines of the last while, kept in memory and in no file,
	// see appLog.
	if recent := theLog.recent.all(); len(recent) > 0 {
		if err := add("logs/recent.log", recent); err != nil {
			return "", err
		}
	}
	if err := z.Close(); err != nil {
		return "", err
	}
	folder, show := m.folder, m.show
	if folder == nil {
		folder = reportFolder
	}
	if show == nil {
		show = showInFolder
	}
	path := freeName(folder(), "Speaker Trail report "+time.Now().Format("2006-01-02 15.04"), ".zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", fmt.Errorf("the report could not be saved: %w", err)
	}
	theLog.line(zerolog.InfoLevel, "report").Int("bytes", buf.Len()).Int("pictures", len(pictures)).Msg("a report of a problem was made")
	m.report.clear(sent)
	_ = show(path)
	// Finder has what was made, and the window it was made in has done
	// its work.
	m.report.closeWindow("report")
	return path, nil
}

// freeName is a path in dir that nothing has yet: the name, or the name
// with a number after it.
func freeName(dir, name, ext string) string {
	path := filepath.Join(dir, name+ext)
	for i := 2; fileThere(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s %d%s", name, i, ext))
	}
	return path
}

func fileThere(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// logName is the name a file of the log has in a report, written like
// every other name there, in small letters with hyphens:
// app-2026-10-09T10-00-00.000.log.gz, as lumberjack keeps an older file,
// is app-2026-10-09-10-00-00.000.log.
func logName(path string) string {
	return strings.ToLower(strings.Replace(strings.TrimSuffix(filepath.Base(path), ".gz"), "T", "-", 1))
}
