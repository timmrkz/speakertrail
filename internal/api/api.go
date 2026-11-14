// Package api answers the interface's requests, in the shapes
// docs/api.md describes. It runs inside the app, behind the window, so
// there is no login: nothing outside the app can reach it.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/store"
)

// Keys keeps the search provider's key, in the macOS keychain in the app
// and in memory in tests.
type Keys interface {
	// Has says whether the key is saved, without reading it.
	Has() bool
	Set(key string) error
}

// API is the handler.
type API struct {
	Store  *store.Store
	Engine *engine.Engine
	Keys   Keys
	// Browsers opens LinkedIn in the right browser profile. Nil where
	// there is no Mac to open it on.
	Browsers Browsers
	Log      *slog.Logger
	mux      *http.ServeMux
}

// New builds the handler.
func New(st *store.Store, e *engine.Engine, keys Keys, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	a := &API{Store: st, Engine: e, Keys: keys, Log: log, mux: http.NewServeMux()}
	m := a.mux
	m.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		send(w, http.StatusOK, map[string]bool{"logged_in": true})
	})
	m.HandleFunc("GET /api/people", a.people)
	m.HandleFunc("GET /api/people/{id}", a.person)
	m.HandleFunc("PATCH /api/people/{id}", a.patchPerson)
	m.HandleFunc("PATCH /api/profiles/{id}", a.patchProfile)
	m.HandleFunc("GET /api/sources", a.searches)
	m.HandleFunc("POST /api/sources", a.addSearch)
	m.HandleFunc("PATCH /api/sources/{id}", a.patchSearch)
	m.HandleFunc("POST /api/sources/{id}/check", a.checkSearch)
	m.HandleFunc("GET /api/exclusions", a.exclusions)
	m.HandleFunc("POST /api/exclusions", a.addExclusion)
	m.HandleFunc("DELETE /api/exclusions/{id}", a.deleteExclusion)
	m.HandleFunc("GET /api/runs", a.runs)
	m.HandleFunc("GET /api/runs/current", a.currentRun)
	m.HandleFunc("POST /api/runs", a.startRun)
	m.HandleFunc("POST /api/runs/{id}/stop", a.stopRun)
	m.HandleFunc("GET /api/runs/{id}", a.run)
	m.HandleFunc("GET /api/settings", a.settings)
	m.HandleFunc("PATCH /api/settings/{key}", a.patchSetting)
	m.HandleFunc("GET /api/keys", a.keys)
	m.HandleFunc("PUT /api/keys/exa", a.setKey)
	m.HandleFunc("GET /api/linkedin", a.linkedin)
	m.HandleFunc("PUT /api/linkedin", a.setLinkedin)
	m.HandleFunc("POST /api/people/{id}/linkedin", a.openLinkedin)
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "No such endpoint") })
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut:
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			fail(w, http.StatusUnsupportedMediaType, "Send JSON with Content-Type application/json")
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	a.mux.ServeHTTP(w, r)
}

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	send(w, status, map[string]string{"error": msg})
}

func (a *API) internal(w http.ResponseWriter, r *http.Request, err error) {
	a.Log.Error("a request failed", "path", r.URL.Path, "error", err)
	fail(w, http.StatusInternalServerError, "Something went wrong. The app's log says what")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "The request is not valid JSON")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "The id must be a number")
		return 0, false
	}
	return id, true
}

// errNotFound ends an update that found nothing to change.
var errNotFound = errors.New("not found")

// update runs fn on the store and answers a failure: 404 for errNotFound,
// 400 for errBad, 500 for the rest. It reports whether fn succeeded.
func (a *API) update(w http.ResponseWriter, r *http.Request, fn func(d *store.Data) error) bool {
	err := a.Store.Update(fn)
	var bad errBad
	switch {
	case err == nil:
		return true
	case errors.Is(err, errNotFound):
		fail(w, http.StatusNotFound, "Not found")
	case errors.As(err, &bad):
		fail(w, http.StatusBadRequest, bad.Error())
	default:
		a.internal(w, r, err)
	}
	return false
}

// errBad is a request that asks for something that cannot be.
type errBad struct{ error }

// ptrTime gives nil for the zero time, so JSON says null.
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
