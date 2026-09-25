// Package server serves the web interface and its JSON API, described in
// docs/api.md.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pipeline is what the server needs to queue work.
type Pipeline interface {
	CheckNow(ctx context.Context, sourceID int64) (int64, error)
	RunNow(ctx context.Context) (runID int64, started bool, err error)
	EnqueueSeed(ctx context.Context, seedID int64) error
}

// Options configure the server.
type Options struct {
	Pool     *pgxpool.Pool
	Pipeline Pipeline
	// UI is the built web interface. It may be nil or empty.
	UI            fs.FS
	PasswordHash  string
	SessionSecret string
	Now           func() time.Time
	Log           *slog.Logger
}

// Server is the HTTP handler.
type Server struct {
	opts     Options
	sessions *sessions
	mux      *http.ServeMux
}

// New returns the server.
func New(opts Options) *Server {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	s := &Server{opts: opts, sessions: newSessions(opts.SessionSecret, opts.PasswordHash, opts.Now), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", s.healthz)

	m.HandleFunc("GET /api/public/config", s.publicConfig)
	m.HandleFunc("GET /api/public/events", s.publicEvents)

	m.HandleFunc("POST /api/login", s.login)
	m.HandleFunc("POST /api/logout", s.logout)
	m.HandleFunc("GET /api/me", s.me)

	p := func(pattern string, h http.HandlerFunc) { m.Handle(pattern, s.private(h)) }
	p("GET /api/stats", s.stats)
	p("GET /api/events", s.events)
	p("PATCH /api/events/{id}", s.patchEvent)
	p("GET /api/people", s.people)
	p("GET /api/people/{id}", s.person)
	p("PATCH /api/people/{id}", s.patchPerson)
	p("PATCH /api/profiles/{id}", s.patchProfile)
	p("GET /api/sources", s.sources)
	p("POST /api/sources", s.addSource)
	p("PATCH /api/sources/{id}", s.patchSource)
	p("POST /api/sources/{id}/check", s.checkSource)
	p("GET /api/runs", s.runs)
	p("POST /api/runs", s.startRun)
	p("GET /api/runs/{id}", s.run)
	p("GET /api/fetches/{id}/{part}", s.fetchPart)
	p("GET /api/seeds", s.seeds)
	p("POST /api/seeds", s.addSeed)
	p("GET /api/settings", s.settings)
	p("PATCH /api/settings/{key}", s.patchSetting)

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "No such endpoint") })
	m.HandleFunc("/", s.ui)
}

// ServeHTTP adds security headers and guards state-changing requests
// against cross-site forms: they must send JSON.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	if isHTTPS(r) {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/logout" &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			fail(w, http.StatusUnsupportedMediaType, "Send JSON with Content-Type application/json")
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) private(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.sessions.loggedIn(r) {
			fail(w, http.StatusUnauthorized, "Log in to see this")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.opts.Pool.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	io.WriteString(w, "ok\n")
}

// ui serves the built interface and falls back to index.html, so every
// path of the single-page app loads.
func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fail(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.opts.UI == nil {
		http.Error(w, "The interface is not built. Run npm run build in web/.", http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" {
		if f, err := s.opts.UI.Open(name); err == nil {
			st, err := f.Stat()
			f.Close()
			if err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.opts.UI, name)
				return
			}
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}
	index, err := fs.ReadFile(s.opts.UI, "index.html")
	if err != nil {
		http.Error(w, "The interface is not built. Run npm run build in web/.", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.enabled() {
		fail(w, http.StatusServiceUnavailable, "The login is not set up. Set UI_PASSWORD_HASH and SESSION_SECRET")
		return
	}
	if !s.sessions.allowAttempt(r) {
		fail(w, http.StatusTooManyRequests, "Too many tries. Wait a minute and try again")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !s.sessions.checkPassword(body.Password) {
		fail(w, http.StatusUnauthorized, "That password is not right")
		return
	}
	s.sessions.setCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"logged_in": s.sessions.loggedIn(r)})
}

// JSON helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeRaw sends JSON that the database already built, without the spaces
// Postgres puts in.
func writeRaw(w http.ResponseWriter, status int, raw []byte) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err == nil {
		raw = buf.Bytes()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(raw)
	w.Write([]byte("\n"))
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "The request body is not valid: "+err.Error())
		return false
	}
	return true
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.opts.Log.Error("request failed", "path", r.URL.Path, "error", err)
	fail(w, http.StatusInternalServerError, "Something went wrong on the server")
}

// sendQuery runs a query that returns one JSON value and sends it.
func (s *Server) sendQuery(w http.ResponseWriter, r *http.Request, status int, sql string, args ...any) {
	var raw []byte
	err := s.opts.Pool.QueryRow(r.Context(), sql, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (raw == nil || string(raw) == "null")) {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeRaw(w, status, raw)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusNotFound, "Not found")
		return 0, false
	}
	return id, true
}
