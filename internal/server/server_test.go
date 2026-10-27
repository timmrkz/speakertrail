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

	// With history: a check took 4 s, a read 20 s, and one read for every
	// check. One check left runs beside others and brings one more read.
	// Reads run alone: two of them, about 40 s.
	exec(`DELETE FROM source_checks`)
	exec(`DELETE FROM event_reads`)
	exec(`INSERT INTO source_checks (source_id, duration_ms, checked_at) VALUES ($1, 4000, $2)`, a, now)
	exec(`INSERT INTO event_reads (event_id, url, duration_ms, read_at) VALUES ($1, 'https://example.org/e/1', 20000, $2)`, ev, now)
	current()
	if s := got.Run.Progress.SecondsLeft; s == nil || *s != 40 {
		t.Errorf("seconds left %v, want 40", s)
	}

	// A portfolio check still to come brings up to 10 lookups. Lookups run
	// four at a time, beside the checks: two checks of 4 s and eleven
	// lookups of 32 s, about 90 s, more than the reads.
	exec(`INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel Hub', 'portfolio', 'https://example.org/hub', 'active')`)
	exec(`INSERT INTO organisations (name, normalised_name, website) VALUES ('Probe Labs', 'probe labs', 'https://probe.example/')`)
	var hub, org int64
	e.pool.QueryRow(ctx, `SELECT id FROM sources WHERE name = 'Beispiel Hub'`).Scan(&hub)
	e.pool.QueryRow(ctx, `SELECT id FROM organisations WHERE name = 'Probe Labs'`).Scan(&org)
	job("check_source", fmt.Sprintf("run:%d:source:%d", run, hub), "queued", fmt.Sprintf(`{"source_id": %d}`, hub))
	job("look_up_startup", fmt.Sprintf("run:%d:lookup:%d", run, org), "running", fmt.Sprintf(`{"organisation_id": %d}`, org))
	exec(`INSERT INTO startup_lookups (organisation_id, url, duration_ms, looked_up_at) VALUES ($1, 'https://probe.example/', 32000, $2)`, org, now)
	code, body := e.do(t, "GET", "/api/runs/current", "")
	if code != 200 || !strings.Contains(body, `"lookups":1`) || !strings.Contains(body, `"lookups_expected":11`) ||
		!strings.Contains(body, `"kind":"lookup","label":"Probe Labs"`) || !strings.Contains(body, `"seconds_left":90`) {
		t.Errorf("progress with lookups: %s", body)
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
	// The search budgets, spent this month. The test has no keys.
	if !strings.Contains(body, `"searches":[{"provider":"exa","used":0,"budget":1000,"set":false},{"provider":"tavily","used":0,"budget":1000,"set":false},{"provider":"brave","used":0,"budget":1000,"set":false}]`) {
		t.Errorf("searches in the stats: %s", body)
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
	if _, body := e.do(t, "GET", "/api/people?filter=founder", ""); !strings.Contains(body, `"counts":{"all":1,"founder":1,"fits":0,"upcoming":1,"profile":1}`) || !strings.Contains(body, "Beispiel Robotics") {
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

	// A portfolio lists startups. It is added as it is, and switches back
	// to a page of events with one change.
	code, body = e.do(t, "POST", "/api/sources", `{"url":"https://hub.example/portfolio","portfolio":true}`)
	if code != 201 || !strings.Contains(body, `"kind":"portfolio"`) {
		t.Errorf("add a portfolio: %d %s", code, body)
	}
	var hub int64
	e.pool.QueryRow(t.Context(), `SELECT id FROM sources WHERE url = 'https://hub.example/portfolio'`).Scan(&hub)
	e.pool.Exec(t.Context(), `UPDATE sources SET next_check_at = now() + interval '9 days' WHERE id = $1`, hub)
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(hub), `{"portfolio":false}`); code != 200 || !strings.Contains(body, `"kind":"listing"`) || !strings.Contains(body, `"next_check_at":null`) {
		t.Errorf("a portfolio back to a listing: %d %s", code, body)
	}
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(hub), `{"portfolio":true}`); code != 200 || !strings.Contains(body, `"kind":"portfolio"`) {
		t.Errorf("a listing to a portfolio: %d %s", code, body)
	}
	e.pool.Exec(t.Context(), `INSERT INTO source_checks (source_id, startups_found, checked_at) VALUES ($1, 0, now())`, hub)
	if code, body := e.do(t, "GET", "/api/sources?q=hub.example", ""); code != 200 || !strings.Contains(body, "The last check found no startups") {
		t.Errorf("an empty portfolio: %d %s", code, body)
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

// Founders of startups that look alive come first, and each says what the
// lookup saw. All names are invented.
func TestPeopleShowWhetherTheirStartupIsActive(t *testing.T) {
	e := setup(t)
	e.login(t)
	ctx := t.Context()
	for _, row := range []struct{ person, company, activity string }{
		{"Anna Anfang", "Gone GmbH", "gone"},
		{"Bea Beispiel", "Quiet GmbH", "quiet"},
		{"Cem Current", "Active GmbH", "active"},
	} {
		var org, person int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO organisations (name, normalised_name, activity, activity_note, last_sign_at)
			VALUES ($1, lower($1), $2, 'a note', '2026-08-01') RETURNING id`, row.company, row.activity).Scan(&org); err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO people (full_name, normalised_name, fit) VALUES ($1, lower($1), 'founder') RETURNING id`, row.person).Scan(&person); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO affiliations (person_id, organisation_id, role) VALUES ($1, $2, 'founder')`, person, org); err != nil {
			t.Fatal(err)
		}
	}
	code, body := e.do(t, "GET", "/api/people?filter=founder", "")
	if code != 200 {
		t.Fatalf("people: %d %s", code, body)
	}
	cem, bea, anna := strings.Index(body, "Cem Current"), strings.Index(body, "Bea Beispiel"), strings.Index(body, "Anna Anfang")
	if !(cem < bea && bea < anna) {
		t.Errorf("order: active %d, quiet %d, gone %d", cem, bea, anna)
	}
	if !strings.Contains(body, `"state":"active"`) || !strings.Contains(body, `"company":"Active GmbH"`) || !strings.Contains(body, `"note":"a note"`) {
		t.Errorf("activity missing: %s", body)
	}

	// A person's sheet links each business's website and imprint, where
	// Tim finds how to reach them. No address or number is stored.
	e.pool.Exec(ctx, `UPDATE organisations SET website = 'https://active.example/', imprint_url = 'https://active.example/impressum' WHERE name = 'Active GmbH'`)
	var cem2 int64
	e.pool.QueryRow(ctx, `SELECT id FROM people WHERE full_name = 'Cem Current'`).Scan(&cem2)
	code, body = e.do(t, "GET", "/api/people/"+itoa(cem2), "")
	flat := strings.ReplaceAll(body, " ", "")
	if code != 200 || !strings.Contains(flat, `"website":"https://active.example/"`) || !strings.Contains(flat, `"imprint_url":"https://active.example/impressum"`) {
		t.Errorf("the business's website and imprint on the sheet: %d %s", code, body)
	}
}

// A source Tim retires by hand is not checked again until he sets it back.
// Before, it kept its old next check and came back in the next run.
func TestARetiredSourceStaysRetired(t *testing.T) {
	e := setup(t)
	e.login(t)
	var id int64
	e.pool.QueryRow(t.Context(), `INSERT INTO sources (name, kind, url, status, next_check_at) VALUES ('Junk', 'listing', 'https://example.org/junk', 'probation', now() - interval '1 day') RETURNING id`).Scan(&id)
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(id), `{"status":"retired"}`); code != 200 {
		t.Fatalf("retire: %d %s", code, body)
	}
	var far bool
	e.pool.QueryRow(t.Context(), `SELECT next_check_at > now() + interval '1 year' FROM sources WHERE id = $1`, id).Scan(&far)
	if !far {
		t.Error("a source retired by hand is due again")
	}
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(id), `{"status":"active"}`); code != 200 || !strings.Contains(body, `"next_check_at":null`) {
		t.Errorf("set back to active: %d %s", code, body)
	}
}

// A run says its true state and what it brought: new people, new fits,
// startups looked up and what failed.
func TestRunsSayHowTheyEnded(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	run := func(ended string, finished bool) int64 {
		var id int64
		var at any
		if finished {
			at = now
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO runs (kind, started_at, finished_at, ended) VALUES ('manual', $1, $2, $3) RETURNING id`,
			now, at, ended).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	finished, stopped, restart, going := run("", false), run("stopped", true), run("restart", true), run("", false)
	exec(`INSERT INTO jobs (kind, key, run_after, created_at, updated_at) VALUES ('check_source', 'run:' || $1::bigint || ':source:1', $2, $2, $2)`, going, now)
	exec(`INSERT INTO people (full_name, normalised_name, fit, first_run_id) VALUES ('Mia Beispiel', 'mia beispiel', 'founder', $1), ('Ole Muster', 'ole muster', 'other', $1)`, finished)
	exec(`INSERT INTO organisations (name, normalised_name) VALUES ('Probe Labs', 'probe labs')`)
	exec(`INSERT INTO startup_lookups (run_id, organisation_id, url, error, looked_up_at) SELECT $1, id, 'https://probe.example/', 'get https://probe.example/: context deadline exceeded', $2 FROM organisations WHERE name = 'Probe Labs'`, finished, now)

	code, body := e.do(t, "GET", "/api/runs", "")
	if code != 200 {
		t.Fatalf("runs: %d %s", code, body)
	}
	var got struct {
		Runs []struct {
			ID            int64  `json:"id"`
			State         string `json:"state"`
			FitsNew       int    `json:"fits_new"`
			LookupsFailed int    `json:"lookups_failed"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{finished: "finished", stopped: "stopped", restart: "interrupted", going: "going"}
	for _, r := range got.Runs {
		if want[r.ID] != r.State {
			t.Errorf("run %d is %q, want %q", r.ID, r.State, want[r.ID])
		}
		if r.ID == finished && (r.FitsNew != 1 || r.LookupsFailed != 1) {
			t.Errorf("the finished run brought %d fits and %d failed lookups, want 1 and 1", r.FitsNew, r.LookupsFailed)
		}
	}
	if len(got.Runs) != 4 {
		t.Errorf("%d runs, want 4", len(got.Runs))
	}
}

// A run's failures are grouped by source, each with the reason in plain
// words and the one action that fixes it. Reads and lookups that failed
// are grouped by reason.
func TestRunFailuresInPlainWords(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var run int64
	e.pool.QueryRow(ctx, `INSERT INTO runs (kind, started_at, finished_at, ended) VALUES ('nightly', $1, $1, 'finished') RETURNING id`, now).Scan(&run)
	source := func(name, status string) int64 {
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, url, status) VALUES ($1, 'listing', $2, $3) RETURNING id`,
			name, "https://"+strings.ToLower(strings.ReplaceAll(name, " ", "-"))+".example/events", status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gone, robots, slow, flaky := source("Beispiel Treff", "retired"), source("Muster Salon", "manual"), source("Probe Abend", "active"), source("Kontroll Runde", "active")
	check := func(src int64, status int, msg string, at time.Time, r any) {
		exec(`INSERT INTO source_checks (run_id, source_id, http_status, error, checked_at) VALUES ($1, $2, $3, $4, $5)`, r, src, status, msg, at)
	}
	check(gone, 404, "get https://beispiel-treff.example/events: status 404", now, run)
	check(robots, 0, "https://muster-salon.example/events: disallowed by robots.txt", now, run)
	check(slow, 0, `get "https://probe-abend.example/events": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, now, run)
	// This one failed in its last three checks, so it is worth retiring.
	for i := range 2 {
		check(flaky, 503, "get https://kontroll-runde.example/events: status 503", now.Add(-time.Duration(i+1)*24*time.Hour), nil)
	}
	check(flaky, 503, "get https://kontroll-runde.example/events: status 503", now, run)
	var ev int64
	e.pool.QueryRow(ctx, `SELECT id FROM events LIMIT 1`).Scan(&ev)
	for range 2 {
		exec(`INSERT INTO event_reads (run_id, event_id, url, error, read_at) VALUES ($1, $2, 'https://example.org/e/129',
			'no language model answers. Is Docker Model Runner on? Turn it on with: docker desktop enable model-runner (dial tcp: connection refused)', $3)`, run, ev, now)
	}

	code, body := e.do(t, "GET", fmt.Sprintf("/api/runs/%d", run), "")
	if code != 200 {
		t.Fatalf("run: %d %s", code, body)
	}
	var got struct {
		Failures []struct {
			What   string `json:"what"`
			Source *struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"source"`
			Reason string `json:"reason"`
			Action string `json:"action"`
			Count  int    `json:"count"`
			Detail string `json:"detail"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, f := range got.Failures {
		name := ""
		if f.Source != nil {
			name = f.Source.Name + " (" + f.Source.Status + ")"
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s, %d, action %q", f.What, name, f.Reason, f.Count, f.Action))
		if f.Detail == "" {
			t.Errorf("%s %s has no detail", f.What, name)
		}
	}
	want := []string{
		`check Beispiel Treff (retired): The page is gone, 1, action ""`,
		`check Kontroll Runde (active): The site had an error of its own, HTTP 503, three checks in a row, 1, action "retire"`,
		`check Muster Salon (manual): robots.txt does not allow the bot, 1, action ""`,
		`check Probe Abend (active): The site did not answer in time, 1, action "check"`,
		`read : The language model did not answer. Is Docker Model Runner on?, 2, action ""`,
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("failures:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// People sort by fit, with the rubric's signals and their passages, and
// the good fits have a filter of their own.
func TestPeopleByFit(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	for _, p := range []struct{ name, headline string }{
		{"Mia Beispiel", "Yoga-Lehrerin und Inhaberin, Studio Beispiel"},
		{"Ole Muster", "Head of Innovation, Beispiel Versicherung"},
		{"Ida Probe", "Life Coach"},
	} {
		if _, err := e.pool.Exec(ctx, `INSERT INTO people (full_name, normalised_name, headline, created_at) VALUES ($1, lower($1), $2, $3)`, p.name, p.headline, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pipeline.ScoreStale(ctx, e.pool); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(t, "GET", "/api/people?sort=fit", "")
	if code != 200 {
		t.Fatalf("people by fit: %d %s", code, body)
	}
	var got struct {
		People []struct {
			Name     string `json:"name"`
			FitScore int    `json:"fit_score"`
			Signals  []struct {
				Key, Label, Passage string
				For                 bool `json:"for"`
			} `json:"signals"`
		} `json:"people"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, p := range got.People {
		order = append(order, fmt.Sprintf("%s %d", p.Name, p.FitScore))
	}
	if strings.Join(order, ", ") != "Mia Beispiel 2, Ida Probe 1, Lea Beispiel 0, Ole Muster -1" {
		t.Errorf("order: %s", strings.Join(order, ", "))
	}
	if s := got.People[0].Signals; len(s) != 2 || s[0].Label != "Works with people" || !s[0].For || s[0].Passage != "Yoga-Lehrerin und Inhaberin, Studio Beispiel" {
		t.Errorf("Mia's signals %+v", s)
	}
	if got.Counts["fits"] != 2 {
		t.Errorf("%d good fits, want 2", got.Counts["fits"])
	}
	if _, body := e.do(t, "GET", "/api/people?filter=fits", ""); strings.Contains(body, "Ole Muster") || !strings.Contains(body, "Ida Probe") {
		t.Errorf("good fits: %s", body)
	}
}

// A source says what its page lists: events, startups or businesses.
func TestSourcesSayWhatTheyList(t *testing.T) {
	e := setup(t)
	e.login(t)
	code, body := e.do(t, "POST", "/api/sources", `{"url":"https://gyms.example/liste","lists":"businesses"}`)
	if code != 201 || !strings.Contains(body, `"kind":"directory"`) {
		t.Fatalf("add a directory: %d %s", code, body)
	}
	var src struct{ ID int64 }
	json.Unmarshal([]byte(body), &src)
	for lists, kind := range map[string]string{"startups": "portfolio", "events": "listing", "businesses": "directory"} {
		code, body := e.do(t, "PATCH", "/api/sources/"+itoa(src.ID), `{"lists":"`+lists+`"}`)
		if code != 200 || !strings.Contains(body, `"kind":"`+kind+`"`) {
			t.Errorf("lists %s: %d %s", lists, code, body)
		}
	}
	// The older way still works.
	if code, body := e.do(t, "PATCH", "/api/sources/"+itoa(src.ID), `{"portfolio":true}`); code != 200 || !strings.Contains(body, `"kind":"portfolio"`) {
		t.Errorf("portfolio true: %d %s", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/sources", `{"url":"https://other.example/","lists":"recipes"}`); code != 400 {
		t.Errorf("an unknown list: %d, want 400", code)
	}
}

// A search is added like a page. Its results are businesses, and it cannot
// be switched to list events.
func TestAddASearch(t *testing.T) {
	e := setup(t)
	e.login(t)
	code, body := e.do(t, "POST", "/api/sources", `{"query":"  Yoga Studio   Bochum "}`)
	if code != 201 || !strings.Contains(body, `"kind":"search_query"`) || !strings.Contains(body, `"query":"Yoga Studio Bochum"`) ||
		!strings.Contains(body, `"city":"Bochum"`) || !strings.Contains(body, `"url":null`) {
		t.Fatalf("add a search: %d %s", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/sources", `{"query":"yoga studio bochum"}`); code != 409 {
		t.Errorf("the same search twice: %d, want 409", code)
	}
	var src struct{ ID int64 }
	json.Unmarshal([]byte(body), &src)
	if code, _ := e.do(t, "PATCH", "/api/sources/"+itoa(src.ID), `{"lists":"events"}`); code != 400 {
		t.Errorf("a search switched to events: %d, want 400", code)
	}
	e.pool.Exec(t.Context(), `INSERT INTO source_checks (source_id, startups_found, events_found) VALUES ($1, 0, 0)`, src.ID)
	if code, body := e.do(t, "GET", "/api/sources?q=bochum", ""); code != 200 || !strings.Contains(body, "The last check found no businesses") {
		t.Errorf("a search in the list: %d %s", code, body)
	}
}

// A post search counts the people in NRW who wrote the posts it found.
// It can be checked by hand like any search.
func TestAPostSearchCountsPeople(t *testing.T) {
	e := setup(t)
	e.login(t)
	var id int64
	if err := e.pool.QueryRow(t.Context(), `INSERT INTO sources (name, kind, query, status) VALUES ('Neues Studio #köln', 'post_search', 'Neues Studio #köln', 'active') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	e.pool.Exec(t.Context(), `INSERT INTO source_checks (source_id, people_found, startups_found, events_found) VALUES ($1, 3, 0, 0)`, id)
	if code, body := e.do(t, "GET", "/api/sources?q=studio", ""); code != 200 || !strings.Contains(body, `"kind":"post_search"`) ||
		!strings.Contains(body, `"people_found":3`) || !strings.Contains(body, `"health":"ok"`) {
		t.Errorf("a post search in the list: %d %s", code, body)
	}
	e.pool.Exec(t.Context(), `INSERT INTO source_checks (source_id, people_found, startups_found, events_found) VALUES ($1, 0, 0, 0)`, id)
	if code, body := e.do(t, "GET", "/api/sources?q=studio", ""); code != 200 || !strings.Contains(body, "The last check found no people in NRW") {
		t.Errorf("a post search that found nobody: %d %s", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/sources/"+itoa(id)+"/check", ""); code == 404 {
		t.Error("a post search cannot be checked by hand")
	}
}

// Keeping or skipping a person is one click and undone by another. Each
// decision remembers the person's signals, so the counts per signal
// survive when a skipped person is deleted later. The stats say how often
// each signal was kept or skipped, and how the top 20 by fit were decided.
func TestKeepsAndSkipsTeachTheRubric(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.login(t)
	ids := map[string]int64{}
	for _, p := range []struct{ name, headline string }{
		{"Mia Beispiel", "Yoga-Lehrerin und Inhaberin, Studio Beispiel"},
		{"Ole Muster", "Head of Innovation, Beispiel Versicherung"},
		{"Ida Probe", "Life Coach"},
	} {
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO people (full_name, normalised_name, headline) VALUES ($1, lower($1), $2) RETURNING id`, p.name, p.headline).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[p.name] = id
	}
	if _, err := pipeline.ScoreStale(ctx, e.pool); err != nil {
		t.Fatal(err)
	}
	decide := func(name, decision string) string {
		t.Helper()
		code, body := e.do(t, "PATCH", "/api/people/"+itoa(ids[name]), `{"decision":"`+decision+`"}`)
		if code != 200 {
			t.Fatalf("%s %s: %d %s", decision, name, code, body)
		}
		return body
	}
	if body := decide("Mia Beispiel", "kept"); !strings.Contains(body, `"decision":"kept"`) {
		t.Errorf("kept: %s", body)
	}
	decide("Ole Muster", "skipped")
	decide("Ida Probe", "kept")
	if body := decide("Ida Probe", ""); !strings.Contains(body, `"decision":""`) {
		t.Errorf("undone: %s", body)
	}
	if code, _ := e.do(t, "PATCH", "/api/people/"+itoa(ids["Ida Probe"]), `{"decision":"maybe"}`); code != 400 {
		t.Errorf("an unknown decision: %d, want 400", code)
	}
	// Skipped people are deleted after a while. Their decision still counts.
	e.pool.Exec(ctx, `DELETE FROM people WHERE id = $1`, ids["Ole Muster"])

	code, body := e.do(t, "GET", "/api/stats", "")
	if code != 200 {
		t.Fatalf("stats: %d %s", code, body)
	}
	var got struct {
		Fit struct {
			Signals []struct {
				Key           string
				Label         string
				Kept, Skipped int
			} `json:"signals"`
			Top struct{ Size, Kept, Skipped, Open int } `json:"top"`
		} `json:"fit"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	counts := map[string]string{}
	for _, s := range got.Fit.Signals {
		counts[s.Key] = fmt.Sprintf("%d/%d", s.Kept, s.Skipped)
	}
	for key, want := range map[string]string{"works_with_people": "1/0", "owner_operator": "1/0", "corporate": "0/1", "author": "0/0"} {
		if counts[key] != want {
			t.Errorf("%s kept/skipped %s, want %s", key, counts[key], want)
		}
	}
	if len(got.Fit.Signals) < 10 || got.Fit.Signals[0].Label != "Works with people" {
		t.Errorf("every signal of the rubric, in its order: %+v", got.Fit.Signals)
	}
	if top := got.Fit.Top; top.Size != 3 || top.Kept != 1 || top.Skipped != 0 || top.Open != 2 {
		t.Errorf("top by fit %+v, want Mia kept, Ida and Lea open", top)
	}
}
