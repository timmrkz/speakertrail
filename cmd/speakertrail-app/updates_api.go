package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// updatesAPI answers the Updates page, at /api/updates, the way the rest
// of the interface is answered. FrameFairy's page calls Wails bindings and
// hears an event, this one asks over HTTP and looks again while something
// moves. See docs/UPDATES.md.
//
//	GET  /api/updates            the state, and the channel list read again at most once a minute
//	POST /api/updates/check      read the list and look for a newer build now
//	POST /api/updates/follow     {"channel": "pr-9"} picks the channel to follow
//	POST /api/updates/relaunch   quits into the build that is ready
//	POST /api/updates/commit     opens the running build's commit on GitHub
type updatesAPI struct {
	// get is the updating once the app has made it, nil before.
	get func() *updating
	// open opens a web page in the browser.
	open func(url string) error
}

func (u updatesAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c := u.get()
	if c == nil {
		answer(w, http.StatusServiceUnavailable, map[string]string{"error": "Not ready yet"})
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/updates":
		go c.refreshList()
		answer(w, http.StatusOK, c.State())
	case "POST /api/updates/check":
		go c.checkNow()
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/updates/follow":
		var body struct {
			Channel string `json:"channel"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		if err := c.Follow(body.Channel); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		answer(w, http.StatusOK, c.State())
	case "POST /api/updates/relaunch":
		if err := c.Restart(); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/updates/commit":
		// Only the running build's commit, never one the interface names.
		link := updates.CommitURL(buildCommit)
		if link == "" {
			answer(w, http.StatusConflict, map[string]string{"error": "This build was made on this Mac and has no commit"})
			return
		}
		if u.open == nil {
			answer(w, http.StatusConflict, map[string]string{"error": errors.New("there is no browser to open it in").Error()})
			return
		}
		if err := u.open(link); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "No such endpoint"})
	}
}

func answer(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
