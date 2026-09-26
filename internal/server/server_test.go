package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/pipeline"
	"github.com/timmrkz/speakertrail/internal/server"
	"github.com/timmrkz/speakertrail/internal/settings"
)

var now = time.Date(2026, 9, 25, 9, 0, 0, 0, extract.Berlin)

type fakePipeline struct {
	checks  []int64
	stopped []int64
	runs    int
}

func (f *fakePipeline) CheckNow(_ context.Context, id int64) (int64, error) {
	f.checks = append(f.checks, id)
	return 42, nil
}

func (f *fakePipeline) StopRun(_ context.Context, id int64) (bool, error) {
	f.stopped = append(f.stopped, id)
	return id == 42, nil
}

func (f *fakePipeline) RunNow(_ context.Context) (int64, bool, error) {
	f.runs++
	return 42, true, nil
}

type env struct {
	pool *pgxpool.Pool
	srv  *httptest.Server
	pipe *fakePipeline
	c    *http.Client
}

const password = "correct horse battery staple"

func setup(t *testing.T) env {
	t.Helper()
	pool := dbtest.New(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	pipe := &fakePipeline{}
	ui := fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>Speaker Trail</title>")},
		"assets/app-abc.js": {Data: []byte("console.log(1)")},
	}
	h := server.New(server.Options{Pool: pool, Pipeline: pipe, UI: ui, PasswordHash: string(hash),
		SessionSecret: "0123456789abcdef0123456789abcdef", Now: func() time.Time { return now }})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	e := env{pool: pool, srv: srv, pipe: pipe, c: &http.Client{Jar: jar}}
	e.seed(t)
	return e
}

// seed stores two events through the real resolver, one kept in Köln with
// a named speaker and one online.
func (e env) seed(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	var src int64
	e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, url, status) VALUES ('Startplatz events', 'listing', 'https://example.org/events', 'active') RETURNING id`).Scan(&src)
	cfg, _ := settings.Load(ctx, e.pool)
	r := &pipeline.Resolver{Pool: e.pool, Rules: pipeline.RulesFrom(cfg), Now: func() time.Time { return now }}
	_, err := r.Resolve(ctx, pipeline.Source{ID: src, URL: "https://example.org/events"}, []extract.Event{
		{Title: "Rheinland Pitch #129", Start: now.Add(3 * 24 * time.Hour), City: "Köln", Venue: "Startplatz", Format: "in_person", Type: "pitch",
			URL: "https://example.org/e/129", Organisers: []extract.Organiser{{Name: "Rheinland Pitch"}},
			People: []extract.Person{{Name: "Lea Beispiel", Role: "pitch", Affiliation: "Founder, Beispiel Robotics", Links: []string{"https://www.linkedin.com/in/lea-beispiel/"}}}},
		{Title: "Scaling Webinar", Start: now.Add(4 * 24 * time.Hour), Format: "online", URL: "https://example.org/e/webinar"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e env) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e env) login(t *testing.T) {
	t.Helper()
	if code, body := e.do(t, "POST", "/api/login", `{"password":"`+password+`"}`); code != 204 {
		t.Fatalf("login: %d %s", code, body)
	}
}

func TestLoginGuardsThePrivateArea(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/api/stats", "/api/events", "/api/people", "/api/sources", "/api/runs", "/api/settings"} {
		if code, _ := e.do(t, "GET", p, ""); code != 401 {
			t.Errorf("GET %s without login: %d, want 401", p, code)
		}
	}
	if code, _ := e.do(t, "POST", "/api/runs", `{}`); code != 401 || e.pipe.runs != 0 {
		t.Errorf("start a run without login: %d", code)
	}
	if _, body := e.do(t, "GET", "/api/me", ""); !strings.Contains(body, `"logged_in":false`) {
		t.Errorf("me: %s", body)
	}
	if code, _ := e.do(t, "POST", "/api/login", `{"password":"wrong"}`); code != 401 {
		t.Errorf("wrong password: %d", code)
	}
	// A form post from another site cannot log in or change anything.
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/login", strings.NewReader("password="+password))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, _ := e.c.Do(req); resp.StatusCode != 415 {
		t.Errorf("form post: %d, want 415", resp.StatusCode)
	}
	e.login(t)
	if code, _ := e.do(t, "GET", "/api/stats", ""); code != 200 {
		t.Errorf("stats after login: %d", code)
	}
	e.do(t, "POST", "/api/logout", "")
	if code, _ := e.do(t, "GET", "/api/stats", ""); code != 401 {
		t.Errorf("stats after logout: %d", code)
	}
}

// A run started by hand records no end. It is finished once none of its
// checks wait any more.
func TestRunsFinishWhenTheirChecksAreDone(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	var run int64
	e.pool.QueryRow(ctx, `INSERT INTO runs (kind, started_at) VALUES ('manual', $1) RETURNING id`, now).Scan(&run)
	if _, err := e.pool.Exec(ctx, `INSERT INTO jobs (kind, key, run_after, created_at, updated_at)
		VALUES ('check_source', 'run:' || $1::bigint || ':source:1', $2, $2, $2)`, run, now); err != nil {
		t.Fatal(err)
	}
	if _, body := e.do(t, "GET", "/api/runs", ""); !strings.Contains(body, `"finished_at":null`) {
		t.Errorf("a run with a waiting check must still be going: %s", body)
	}
	e.pool.Exec(ctx, `UPDATE jobs SET status = 'done'`)
	if _, body := e.do(t, "GET", "/api/runs", ""); strings.Contains(body, `"finished_at":null`) {
		t.Errorf("a run without waiting checks must be finished: %s", body)
	}
}

// A going run says how far it is, what runs now and about how long is left,
// measured from earlier checks and reads.
func TestRunProgress(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel Events', 'listing', 'https://example.org/a', 'active'), ('Muster Meetups', 'listing', 'https://example.org/b', 'active')`)
	exec(`INSERT INTO events (title, starts_at, canonical_url, fit) VALUES ('Pitch Abend', $1, 'https://example.org/e/1', 'kept')`, now.Add(72*time.Hour))
	exec(`INSERT INTO runs (kind, started_at) VALUES ('manual', $1)`, now)
	job := func(kind, key, status, payload string) {
		exec(`INSERT INTO jobs (kind, key, status, payload, run_after, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $5, $5)`,
			kind, key, status, payload, now)
	}
	var a, b, ev, run int64
	e.pool.QueryRow(ctx, `SELECT id FROM sources WHERE name = 'Beispiel Events'`).Scan(&a)
	e.pool.QueryRow(ctx, `SELECT id FROM sources WHERE name = 'Muster Meetups'`).Scan(&b)
	e.pool.QueryRow(ctx, `SELECT id FROM events WHERE title = 'Pitch Abend'`).Scan(&ev)
	e.pool.QueryRow(ctx, `SELECT max(id) FROM runs`).Scan(&run)
	job("check_source", fmt.Sprintf("run:%d:source:%d", run, a), "done", fmt.Sprintf(`{"source_id": %d}`, a))
	job("check_source", fmt.Sprintf("run:%d:source:%d", run, b), "running", fmt.Sprintf(`{"source_id": %d}`, b))
	job("read_event", fmt.Sprintf("run:%d:read:%d", run, ev), "queued", fmt.Sprintf(`{"event_id": %d}`, ev))

	var got struct {
		Run *struct {
			Progress *struct {
				Checks, ChecksDone, Reads, ReadsDone int
				Now                                  []struct{ Kind, Label string }
				SecondsLeft                          *float64 `json:"seconds_left"`
			}
		}
	}
	current := func() {
		t.Helper()
		code, body := e.do(t, "GET", "/api/runs/current", "")
		if code != 200 {
			t.Fatalf("current run: %d %s", code, body)
		}
		// The API writes snake case, the struct reads it through lower case.
		body = strings.NewReplacer(`"checks_done"`, `"checksdone"`, `"reads_done"`, `"readsdone"`).Replace(body)
		got.Run = nil
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
	}
	current()
	pr := got.Run.Progress
	if pr == nil || pr.Checks != 2 || pr.ChecksDone != 1 || pr.Reads != 1 || pr.ReadsDone != 0 {
		t.Fatalf("progress %+v", pr)
	}
	if len(pr.Now) != 1 || pr.Now[0].Kind != "check" || pr.Now[0].Label != "Muster Meetups" {
		t.Errorf("running now %+v", pr.Now)
	}
	if pr.SecondsLeft != nil {
		t.Errorf("no history, no estimate, got %v", *pr.SecondsLeft)
	}

	// With history: a check took 4 s, a read 20 s. One check left runs
	// beside others, the read runs alone: about 20 s.
	exec(`DELETE FROM source_checks`)
	exec(`DELETE FROM event_reads`)
	exec(`INSERT INTO source_checks (source_id, duration_ms, checked_at) VALUES ($1, 4000, $2)`, a, now)
	exec(`INSERT INTO event_reads (event_id, url, duration_ms, read_at) VALUES ($1, 'https://example.org/e/1', 20000, $2)`, ev, now)
	current()
	if s := got.Run.Progress.SecondsLeft; s == nil || *s != 20 {
		t.Errorf("seconds left %v, want 20", s)
	}

	// A finished run is not current.
	exec(`UPDATE jobs SET status = 'done'`)
	current()
	if got.Run != nil {
		t.Errorf("a finished run is still current: %+v", got.Run)
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	e := setup(t)
	for range 5 {
		e.do(t, "POST", "/api/login", `{"password":"guess"}`)
	}
	if code, _ := e.do(t, "POST", "/api/login", `{"password":"`+password+`"}`); code != 429 {
		t.Errorf("sixth try within a minute: %d, want 429", code)
	}
}

func TestPublicCalendar(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	// Closed by default. The config still answers, so the front page can
	// say so, and it gives away no cities.
	if code, _ := e.do(t, "GET", "/api/public/events", ""); code != 404 {
		t.Errorf("public events while closed: %d", code)
	}
	if code, body := e.do(t, "GET", "/api/public/config", ""); code != 200 ||
		!strings.Contains(body, `"public_calendar":false`) || !strings.Contains(body, `"owner":false`) || !strings.Contains(body, `"cities":[]`) {
		t.Errorf("config while closed: %d %s", code, body)
	}
	// Tim sees the calendar before it opens.
	e.login(t)
	if code, body := e.do(t, "GET", "/api/public/events", ""); code != 200 || !strings.Contains(body, `"events":[`) {
		t.Errorf("owner preview while closed: %d %s", code, body)
	}
	if _, body := e.do(t, "GET", "/api/public/config", ""); !strings.Contains(body, `"owner":true`) {
		t.Errorf("config for the owner: %s", body)
	}
	e.do(t, "POST", "/api/logout", "")
	e.pool.Exec(ctx, `UPDATE settings SET value = 'true' WHERE key = 'public_calendar'`)

	code, body := e.do(t, "GET", "/api/public/events", "")
	if code != 200 {
		t.Fatalf("public events: %d %s", code, body)
	}
	var got struct {
		Events []struct {
			Title      string   `json:"title"`
			Venue      string   `json:"venue"`
			Organisers []string `json:"organisers"`
			People     []any    `json:"people"`
			StartsAt   string   `json:"starts_at"`
		} `json:"events"`
	}
	json.Unmarshal([]byte(body), &got)
	if len(got.Events) != 1 || got.Events[0].Title != "Rheinland Pitch #129" {
		t.Fatalf("public events %s", body)
	}
	ev := got.Events[0]
	if ev.Venue != "Startplatz" || len(ev.Organisers) != 1 || len(ev.People) != 0 {
		t.Errorf("public event %+v, people must stay hidden", ev)
	}
	if !strings.HasSuffix(ev.StartsAt, "+00:00") && !strings.HasSuffix(ev.StartsAt, "Z") {
		t.Errorf("starts_at %q is not UTC", ev.StartsAt)
	}
	if strings.Contains(body, "Lea Beispiel") || strings.Contains(body, "linkedin") {
		t.Error("a name or profile leaked into the public calendar")
	}

	e.pool.Exec(ctx, `UPDATE settings SET value = 'true' WHERE key = 'public_show_people'`)
	if _, body := e.do(t, "GET", "/api/public/events?city=köln", ""); !strings.Contains(body, "Lea Beispiel") {
		t.Errorf("names should show once the setting allows it: %s", body)
	}
	if _, body := e.do(t, "GET", "/api/public/events?city=Bonn", ""); !strings.Contains(body, `"events":[]`) {
		t.Errorf("city filter: %s", body)
	}
	if _, body := e.do(t, "GET", "/api/public/config", ""); !strings.Contains(body, `"cities":["Köln"]`) {
		t.Errorf("config: %s", body)
	}
}

func TestPrivateAPI(t *testing.T) {
	e := setup(t)
	e.login(t)

	_, body := e.do(t, "GET", "/api/stats", "")
	var stats struct {
		Totals struct {
			People, Organisations, Profiles int
			EventsKeptUpcoming              int            `json:"events_kept_upcoming"`
			Sources                         map[string]int `json:"sources"`
		} `json:"totals"`
		Last7 map[string]int   `json:"last_7_days"`
		Week  []map[string]any `json:"weekly"`
	}
	if err := json.Unmarshal([]byte(body), &stats); err != nil {
		t.Fatalf("stats: %v %s", err, body)
	}
	if stats.Totals.People != 1 || stats.Totals.EventsKeptUpcoming != 1 || stats.Totals.Profiles != 1 || stats.Totals.Sources["active"] != 1 {
		t.Errorf("stats totals %+v", stats.Totals)
	}
	if len(stats.Week) != 8 {
		t.Errorf("%d weeks, want 8", len(stats.Week))
	}

	_, body = e.do(t, "GET", "/api/events?fit=all", "")
	if !strings.Contains(body, `"fit_reason":"online"`) || !strings.Contains(body, `"name":"Lea Beispiel"`) {
		t.Errorf("events: %s", body)
	}
	var id int64
	e.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE title = 'Rheinland Pitch #129'`).Scan(&id)
	code, body := e.do(t, "PATCH", "/api/events/"+itoa(id), `{"fit":"dropped"}`)
	if code != 200 || !strings.Contains(body, `"fit":"dropped"`) || !strings.Contains(body, `"fit_manual":true`) {
		t.Errorf("patch event: %d %s", code, body)
	}

	_, body = e.do(t, "GET", "/api/people?q=beispiel", "")
	if !strings.Contains(body, `"headline":"Founder, Beispiel Robotics"`) || !strings.Contains(body, `"next_appearance":{`) {
		t.Errorf("people: %s", body)
	}
	// A title that says founder makes a founder, and the counts show every
	// filter, so an empty one never hides the rest.
	if _, body := e.do(t, "GET", "/api/people?filter=founder", ""); !strings.Contains(body, `"counts":{"all":1,"founder":1,"upcoming":1,"profile":1}`) || !strings.Contains(body, "Beispiel Robotics") {
		t.Errorf("founders: %s", body)
	}
	var pid, prof int64
	e.pool.QueryRow(t.Context(), `SELECT id FROM people`).Scan(&pid)
	e.pool.QueryRow(t.Context(), `SELECT id FROM profiles`).Scan(&prof)
	if code, _ := e.do(t, "PATCH", "/api/profiles/"+itoa(prof), `{"review":"confirmed"}`); code != 204 {
		t.Errorf("confirm profile: %d", code)
	}
	code, body = e.do(t, "PATCH", "/api/people/"+itoa(pid), `{"notes":"Met at the pitch"}`)
	if code != 200 || !strings.Contains(body, `"notes": "Met at the pitch"`) && !strings.Contains(body, `"notes":"Met at the pitch"`) {
		t.Errorf("patch person: %d %s", code, body)
	}
	if !strings.Contains(body, `"review": "confirmed"`) && !strings.Contains(body, `"review":"confirmed"`) {
		t.Errorf("confirmed profile missing: %s", body)
	}

	code, body = e.do(t, "POST", "/api/sources", `{"url":"https://www.linkedin.com/company/x"}`)
	if code != 400 {
		t.Errorf("adding LinkedIn as a source: %d %s", code, body)
	}
	code, body = e.do(t, "POST", "/api/sources", `{"url":"https://www.meetup.com/beispiel-group/events/1/"}`)
	if code != 201 || !strings.Contains(body, `"url":"https://www.meetup.com/beispiel-group/"`) || !strings.Contains(body, `"health":"never"`) {
		t.Errorf("add source: %d %s", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/sources", `{"url":"https://www.meetup.com/beispiel-group/"}`); code != 409 {
		t.Errorf("adding a source twice: %d", code)
	}
	var sid int64
	e.pool.QueryRow(t.Context(), `SELECT id FROM sources WHERE name = 'Startplatz events'`).Scan(&sid)
	if code, _ := e.do(t, "POST", "/api/sources/"+itoa(sid)+"/check", `{}`); code != 202 || len(e.pipe.checks) != 1 {
		t.Errorf("check now: %d %v", code, e.pipe.checks)
	}
	if code, body := e.do(t, "POST", "/api/runs", `{}`); code != 202 || e.pipe.runs != 1 || !strings.Contains(body, `"started":true`) {
		t.Errorf("start a run: %d %s", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/runs/42/stop", `{}`); code != 204 {
		t.Errorf("stop a run: %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/runs/7/stop", `{}`); code != 409 {
		t.Errorf("stop a run that ended: %d", code)
	}
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(sid), `{"status":"paused"}`); code != 400 {
		t.Errorf("bad status: %d %s", code, body)
	}

	if code, body := e.do(t, "POST", "/api/sources", `{"url":"https://www.facebook.com/beispielgruppe/"}`); code != 400 || !strings.Contains(body, "Facebook") {
		t.Errorf("a Facebook page as a source: %d %s", code, body)
	}

	if code, body := e.do(t, "PATCH", "/api/settings/collect_ahead_days", `{"value":"thirty"}`); code != 400 || !strings.Contains(body, "number") {
		t.Errorf("setting with the wrong type: %d %s", code, body)
	}
	if code, body := e.do(t, "PATCH", "/api/settings/collect_ahead_days", `{"value":45}`); code != 200 || !strings.Contains(body, `"value":45`) {
		t.Errorf("setting: %d %s", code, body)
	}
}

func TestStoredPagesAreServedAsText(t *testing.T) {
	e := setup(t)
	e.login(t)
	var id int64
	e.pool.QueryRow(t.Context(), `INSERT INTO fetches (url, mode, html, visible_text) VALUES ('https://example.org', 'http', '<script>alert(1)</script>', 'Hello') RETURNING id`).Scan(&id)
	resp, err := e.c.Get(e.srv.URL + "/api/fetches/" + itoa(id) + "/html")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") || resp.Header.Get("Content-Security-Policy") != "sandbox" {
		t.Errorf("stored HTML served as %q with CSP %q", ct, resp.Header.Get("Content-Security-Policy"))
	}
	if code, _ := e.do(t, "GET", "/api/fetches/"+itoa(id)+"/screenshot", ""); code != 404 {
		t.Errorf("missing screenshot: %d", code)
	}
}

func TestInterfaceFallsBackToIndex(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/", "/app", "/app/people", "/login"} {
		code, body := e.do(t, "GET", p, "")
		if code != 200 || !strings.Contains(body, "<title>Speaker Trail</title>") {
			t.Errorf("GET %s: %d", p, code)
		}
	}
	if code, _ := e.do(t, "GET", "/assets/app-abc.js", ""); code != 200 {
		t.Errorf("asset: %d", code)
	}
	if code, _ := e.do(t, "GET", "/assets/missing.js", ""); code != 404 {
		t.Errorf("missing asset: %d", code)
	}
	if code, _ := e.do(t, "GET", "/api/nothing", ""); code != 404 {
		t.Errorf("unknown API path: %d", code)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
