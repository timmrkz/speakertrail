package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/timmrkz/speakertrail/internal/store"
)

// logAPI answers the window about the app's log, the way the rest of the
// interface is answered. FrameFairy's window calls Wails bindings, Said,
// Acted, AppLog, WatchAppLog, ShowAppLog and RemoveAppLog, and this one
// asks over HTTP, the way Tim chose for every call. See docs/LOGGING.md.
//
//	POST   /api/log/said    [{"at":…,"level":"warn","act":"a3","text":"…"}] the window's lines
//	POST   /api/log/acted   {"id":"a3","at":…,"what":"pressed button \"Run\""} the person's act
//	GET    /api/log         where the log is, its files, size and lines, the last toasts
//	POST   /api/log/watch   {"on": true} while Settings is open, the event applog follows the log
//	POST   /api/log/show    shows the log in Finder
//	DELETE /api/log         removes every file of the log, after Settings asked
type logAPI struct {
	log *appLog
	// show shows a file in its folder, in Finder on a Mac.
	show func(path string) error
	// unnamed gives what takes every person's name and handle out of the
	// window's lines and acts, store.Store.Unnamed, so no tooltip or
	// message the window forgot to mark can name a person in the log,
	// issue #104.
	unnamed func() func(string) string
}

// newLogAPI is the log's handler as the app has it, with the guard that
// takes the names of the people in st out of what the window says.
func newLogAPI(log *appLog, st *store.Store, show func(path string) error) logAPI {
	return logAPI{log: log, show: show, unnamed: st.Unnamed}
}

// saidMost is how much the window may send at once: saidLines lines of
// saidLineMax characters, with room for their JSON.
const saidMost = 2 << 20

func (l logAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method + " " + r.URL.Path {
	case "POST /api/log/said":
		var lines []SaidLine
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, saidMost)).Decode(&lines); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		clean := l.unnamed()
		for i := range lines[:min(len(lines), saidLines)] {
			lines[i].Text = clean(lines[i].Text)
		}
		l.log.said(lines)
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/log/acted":
		var a Act
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&a); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		a.What = l.unnamed()(a.What)
		l.log.acted(a)
		w.WriteHeader(http.StatusNoContent)
	case "GET /api/log":
		answer(w, http.StatusOK, l.log.status())
	case "POST /api/log/watch":
		var body struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		l.log.watchFor(body.On)
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/log/show":
		path := l.log.status().Path
		if path == "" {
			answer(w, http.StatusConflict, map[string]string{"error": errors.New("the app keeps no log yet").Error()})
			return
		}
		if err := l.show(path); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "DELETE /api/log":
		st, err := l.log.removed()
		if err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		answer(w, http.StatusOK, st)
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "No such endpoint"})
	}
}

// showInFolder shows a file in its folder: selected in Finder on a Mac,
// the folder opened elsewhere. It waits for nothing, and nothing is left
// behind once the program it starts ends.
func showInFolder(path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", "-R", path)
	} else {
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
