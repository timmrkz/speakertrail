package pipeline_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/llm"
	"github.com/timmrkz/speakertrail/internal/pipeline"
	"github.com/timmrkz/speakertrail/internal/queue"
)

// The test world is late September 2026, like the first run.
var now = time.Date(2026, 9, 25, 5, 0, 0, 0, extract.Berlin)

type site struct {
	mu    sync.Mutex
	pages map[string]string
	srv   *httptest.Server
}

func newSite(t *testing.T, robots string) *site {
	s := &site{pages: map[string]string{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, robots)
			return
		}
		s.mu.Lock()
		body, ok := s.pages[r.URL.Path]
		if strings.HasSuffix(r.Host, ".test") {
			body, ok = s.pages["http://"+r.Host+r.URL.Path]
		}
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *site) set(path, body string) {
	s.mu.Lock()
	s.pages[path] = body
	s.mu.Unlock()
}

func ldPage(events ...string) string {
	return `<html><head><script type="application/ld+json">[` + strings.Join(events, ",") + `]</script></head>
<body><h1>Events</h1><a href="https://www.meetup.com/beispiel-founders-koeln/events/123/">also on Meetup</a></body></html>`
}

func ldEvent(name, start, city, desc string) string {
	return fmt.Sprintf(`{"@context":"https://schema.org","@type":"Event","name":%q,"startDate":%q,
		"url":"https://example.org/e/%s","description":%q,
		"location":{"@type":"Place","name":"Startplatz","address":{"@type":"PostalAddress","addressLocality":%q}},
		"organizer":{"@type":"Organization","name":"Rheinland Pitch"}}`,
		name, start, pipeline.NormaliseTitle(name), desc, city)
}

type env struct {
	pool *pgxpool.Pool
	p    *pipeline.Pipeline
	site *site
}

func setup(t *testing.T, robots string) env {
	t.Helper()
	pool := dbtest.New(t)
	s := newSite(t, robots)
	q := queue.New(pool, queue.Options{Now: func() time.Time { return now }})
	// Tests never touch a live website. Any other host fails the test.
	lo := &localOnly{local: s.srv.Listener.Addr().String()}
	t.Cleanup(func() {
		if hosts := lo.tried(); len(hosts) > 0 {
			t.Errorf("the test tried to reach live websites: %v", hosts)
		}
	})
	p := &pipeline.Pipeline{Pool: pool, Fetcher: fetch.New(fetch.Options{Client: &http.Client{Transport: lo, Timeout: 10 * time.Second}}),
		Queue: q, Now: func() time.Time { return now }}
	return env{pool: pool, p: p, site: s}
}

// localOnly lets requests reach the test's own server and refuses and
// records every other host. Invented hosts ending in .test go to the
// test's server too, which answers them from pages set as
// "http://host.test/path".
type localOnly struct {
	mu    sync.Mutex
	hosts []string
	local string
}

func (l *localOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if h := r.URL.Hostname(); h == "127.0.0.1" || h == "localhost" || h == "::1" {
		return http.DefaultTransport.RoundTrip(r)
	}
	if strings.HasSuffix(r.URL.Hostname(), ".test") {
		local := r.Clone(r.Context())
		local.Host = r.URL.Host
		local.URL.Host = l.local
		resp, err := http.DefaultTransport.RoundTrip(local)
		if resp != nil {
			resp.Request = r
		}
		return resp, err
	}
	l.mu.Lock()
	l.hosts = append(l.hosts, r.URL.Host)
	l.mu.Unlock()
	return nil, fmt.Errorf("tests never touch a live website: %s", r.URL.Host)
}

func (l *localOnly) tried() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.hosts...)
}

func (e env) addSource(t *testing.T, path, status string) int64 {
	t.Helper()
	var id int64
	err := e.pool.QueryRow(t.Context(), `INSERT INTO sources (name, kind, url, status, city) VALUES ($1, 'listing', $2, $3, 'Köln') RETURNING id`,
		"Test source "+path, e.site.srv.URL+path, status).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) one(t *testing.T, sql string, args ...any) any {
	t.Helper()
	var v any
	if err := e.pool.QueryRow(t.Context(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func (e env) count(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	return e.one(t, "SELECT count(*) FROM "+sql, args...).(int64)
}

func TestCheckSourceStoresEventsPeopleAndProvenance(t *testing.T) {
	e := setup(t, "User-agent: *\nAllow: /\n")
	ctx := t.Context()
	e.site.set("/events", ldPage(
		ldEvent("Rheinland Pitch #129", "2026-09-28T18:00:00+02:00", "Köln",
			"Finalisten:\n- Lea Beispiel (Beispiel Robotics)\n- Jonas Muster, Gründer bei Kaffeekreis\nModeration: Nicola"),
		ldEvent("Online Sales Webinar", "2026-09-29T18:00:00+02:00", "Köln", ""),
		ldEvent("Founder Night Berlin", "2026-09-30T18:00:00+02:00", "Berlin", "Speaker: Mia Fern (Fern GmbH)"),
		ldEvent("Pitch Night next year", "2027-03-01T18:00:00+01:00", "Köln", ""),
	))
	src := e.addSource(t, "/events", "candidate")
	run, err := e.p.StartRun(ctx, "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.p.CheckSource(ctx, src, run); err != nil {
		t.Fatal(err)
	}

	// The event beyond the 30 day horizon is not stored.
	if n := e.count(t, "events"); n != 3 {
		t.Fatalf("%d events stored, want 3", n)
	}
	fits := map[string]string{}
	rows, _ := e.pool.Query(ctx, `SELECT title, fit || ': ' || fit_reason FROM events`)
	for rows.Next() {
		var title, fit string
		rows.Scan(&title, &fit)
		fits[title] = fit
	}
	rows.Close()
	want := map[string]string{
		"Rheinland Pitch #129": "kept: keep word: pitch",
		"Online Sales Webinar": "dropped: drop word: webinar",
		"Founder Night Berlin": "dropped: outside the region: Berlin",
	}
	for title, w := range want {
		if fits[title] != w {
			t.Errorf("%s: fit %q, want %q", title, fits[title], w)
		}
	}

	// A first name alone is too easily a company or a place, so Nicola is
	// not stored.
	if n := e.count(t, "people"); n != 3 {
		t.Errorf("%d people, want 3 (two on the pitch, one in Berlin)", n)
	}
	if role := e.one(t, `SELECT a.role FROM appearances a JOIN people p ON p.id = a.person_id WHERE p.full_name = 'Lea Beispiel'`); role != "pitch" {
		t.Errorf("Lea Beispiel's role %v", role)
	}
	if org := e.one(t, `SELECT o.name || '/' || af.role FROM affiliations af JOIN organisations o ON o.id = af.organisation_id
		JOIN people p ON p.id = af.person_id WHERE p.full_name = 'Jonas Muster'`); org != "Kaffeekreis/founder" {
		t.Errorf("Jonas Muster's affiliation %v", org)
	}
	if n := e.count(t, "organisings og JOIN organisations o ON o.id = og.organisation_id WHERE o.name = 'Startplatz' AND og.role = 'venue'"); n != 3 {
		t.Errorf("Startplatz is the venue of %d events, want 3", n)
	}
	if n := e.count(t, "sightings WHERE source_id = $1 AND is_new", src); n < 7 {
		t.Errorf("only %d new sightings recorded", n)
	}

	// The source advanced: its first check kept an event with people.
	var status, mode string
	var checks, empty int
	e.pool.QueryRow(ctx, `SELECT status, fetch_mode, checks, empty_checks_in_row FROM sources WHERE id = $1`, src).Scan(&status, &mode, &checks, &empty)
	if status != "active" || mode != "http" || checks != 1 || empty != 0 {
		t.Errorf("source status %s mode %s checks %d empty %d", status, mode, checks, empty)
	}
	// The Meetup link became a candidate source for the whole group.
	if n := e.count(t, "sources WHERE url = 'https://www.meetup.com/beispiel-founders-koeln/' AND status = 'candidate' AND discovered_from_source_id = $1", src); n != 1 {
		t.Error("the linked Meetup group was not added as a candidate")
	}
	var found, kept, newEvents, people int
	e.pool.QueryRow(ctx, `SELECT events_found, events_kept, events_new, people_found FROM source_checks WHERE run_id = $1`, run).Scan(&found, &kept, &newEvents, &people)
	if found != 3 || kept != 1 || newEvents != 3 || people != 3 {
		t.Errorf("check record found %d kept %d new %d people %d", found, kept, newEvents, people)
	}
	if n := e.count(t, "fetches"); n != 1 {
		t.Errorf("%d fetches stored", n)
	}

	// Tim drops the pitch by hand. The next check finds the same page and
	// changes nothing: no new events, no new people, Tim's decision stays.
	e.pool.Exec(ctx, `UPDATE events SET fit = 'dropped', fit_reason = 'not a fit', fit_manual = true WHERE title = 'Rheinland Pitch #129'`)
	if err := e.p.CheckSource(ctx, src, run); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, "events"); n != 3 {
		t.Errorf("%d events after the second check", n)
	}
	if n := e.count(t, "people"); n != 3 {
		t.Errorf("%d people after the second check", n)
	}
	if fit := e.one(t, `SELECT fit FROM events WHERE title = 'Rheinland Pitch #129'`); fit != "dropped" {
		t.Error("the automatic rules overwrote Tim's decision")
	}
	var newest int
	e.pool.QueryRow(ctx, `SELECT events_new + people_new FROM source_checks ORDER BY id DESC LIMIT 1`).Scan(&newest)
	if newest != 0 {
		t.Errorf("second check counted %d new rows", newest)
	}
}

func TestPeopleAreMergedCarefully(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	e.site.set("/a", ldPage(ldEvent("Founder Stories Köln", "2026-09-28T18:00:00+02:00", "Köln",
		"Speakers: Lea Beispiel (Beispiel Robotics), Anna Doppel (Firma Eins)\nHost: Nicola")))
	e.site.set("/b", ldPage(ldEvent("Female Founders Night Bonn", "2026-10-02T18:00:00+02:00", "Bonn",
		"Speakers: Lea Beispiel, Anna Doppel (Firma Zwei)\nHost: Nicola")))
	if err := e.p.CheckSource(ctx, e.addSource(t, "/a", "active"), 0); err != nil {
		t.Fatal(err)
	}
	if err := e.p.CheckSource(ctx, e.addSource(t, "/b", "active"), 0); err != nil {
		t.Fatal(err)
	}
	// Lea is one person: no organisation contradicts. The two Annas name
	// different companies, so they stay apart. A first name alone is not
	// stored at all.
	for name, want := range map[string]int64{"Lea Beispiel": 1, "Anna Doppel": 2, "Nicola": 0} {
		if n := e.count(t, "people WHERE full_name = $1", name); n != want {
			t.Errorf("%s: %d people, want %d", name, n, want)
		}
	}
	if n := e.count(t, "appearances a JOIN people p ON p.id = a.person_id WHERE p.full_name = 'Lea Beispiel'"); n != 2 {
		t.Errorf("Lea Beispiel has %d appearances, want 2", n)
	}
}

func TestBlockedSiteBecomesManual(t *testing.T) {
	e := setup(t, "User-agent: *\nDisallow: /\n")
	e.site.set("/events", ldPage())
	src := e.addSource(t, "/events", "active")
	if err := e.p.CheckSource(t.Context(), src, 0); err != nil {
		t.Fatalf("a blocked site should not be retried: %v", err)
	}
	if s := e.one(t, `SELECT status || ': ' || notes FROM sources WHERE id = $1`, src); s != "manual: robots.txt does not allow the bot" {
		t.Errorf("source %v", s)
	}
	if n := e.count(t, "source_checks WHERE source_id = $1 AND error <> ''", src); n != 1 {
		t.Error("the failed check was not recorded")
	}
}

func TestLifecycle(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	e.site.set("/empty", "<html><body><h1>Nothing planned yet</h1><p>"+strings.Repeat("Check back soon for new dates. ", 20)+"</p></body></html>")

	probation := e.addSource(t, "/empty", "probation")
	e.pool.Exec(ctx, `UPDATE sources SET status_changed_at = $2 WHERE id = $1`, probation, now.Add(-22*24*time.Hour))
	if err := e.p.CheckSource(ctx, probation, 0); err != nil {
		t.Fatal(err)
	}
	if s := e.one(t, `SELECT status FROM sources WHERE id = $1`, probation); s != "retired" {
		t.Errorf("probation without events for 3 weeks: status %v, want retired", s)
	}
	next := e.one(t, `SELECT next_check_at FROM sources WHERE id = $1`, probation).(time.Time)
	if next.Sub(now) < 20*24*time.Hour {
		t.Errorf("retired source is checked again already at %v", next)
	}

	e.pool.Exec(ctx, `UPDATE sources SET url = url || '?x', status = 'active', empty_checks_in_row = 3 WHERE id = $1`, probation)
	if err := e.p.CheckSource(ctx, probation, 0); err != nil {
		t.Fatal(err)
	}
	if s := e.one(t, `SELECT status FROM sources WHERE id = $1`, probation); s != "retired" {
		t.Errorf("active after a 4th empty check: status %v, want retired", s)
	}
}

func TestEnqueueDue(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	for i, status := range []string{"active", "probation", "manual", "retired", "active"} {
		id := e.addSource(t, fmt.Sprintf("/s%d", i), status)
		if i == 3 || i == 4 {
			e.pool.Exec(ctx, `UPDATE sources SET next_check_at = $2 WHERE id = $1`, id, now.Add(24*time.Hour))
		}
	}
	for i := range 12 {
		e.addSource(t, fmt.Sprintf("/c%d", i), "candidate")
	}
	e.pool.Exec(ctx, `INSERT INTO seeds (input) VALUES ('https://www.meetup.com/beispiel-group/events/99/')`)
	run, _ := e.p.StartRun(ctx, "nightly")
	n, err := e.p.EnqueueDue(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	// Active and probation that are due, plus 5 new candidates.
	if n != 7 {
		t.Errorf("%d sources queued, want 7", n)
	}
	if c := e.count(t, "jobs WHERE kind = 'process_seed'"); c != 1 {
		t.Errorf("%d seed jobs", c)
	}
	if c := e.count(t, "jobs WHERE kind = 'prune'"); c != 1 {
		t.Errorf("%d prune jobs", c)
	}
	// Queueing twice in one run changes nothing.
	if _, err := e.p.EnqueueDue(ctx, run); err != nil {
		t.Fatal(err)
	}
	if c := e.count(t, "jobs WHERE kind = 'check_source'"); c != 7 {
		t.Errorf("%d check jobs after queueing twice", c)
	}

	// The next run replaces checks the last one left behind.
	next, _ := e.p.StartRun(ctx, "nightly")
	if _, err := e.p.EnqueueDue(ctx, next); err != nil {
		t.Fatal(err)
	}
	if c := e.count(t, "jobs WHERE kind = 'check_source' AND status = 'queued' AND key LIKE $1", fmt.Sprintf("run:%d:%%", run)); c != 0 {
		t.Errorf("%d checks of the old run are still queued", c)
	}
}

// Two clicks on Start a run, or two people at once, start one run.
func TestRunNowStartsOneRunAtATime(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	for i := range 3 {
		e.addSource(t, fmt.Sprintf("/s%d", i), "active")
	}
	const callers = 8
	ids := make([]int64, callers)
	started := make([]bool, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			id, ok, err := e.p.RunNow(ctx)
			if err != nil {
				t.Error(err)
			}
			ids[i], started[i] = id, ok
		})
	}
	wg.Wait()
	n := 0
	for i := range callers {
		if started[i] {
			n++
		}
		if ids[i] != ids[0] {
			t.Errorf("caller %d got run %d, caller 0 got run %d", i, ids[i], ids[0])
		}
	}
	if n != 1 {
		t.Errorf("%d runs started, want 1", n)
	}
	if c := e.count(t, "runs WHERE kind = 'manual'"); c != 1 {
		t.Errorf("%d runs recorded", c)
	}
	if c := e.count(t, "jobs WHERE kind = 'check_source' AND status = 'queued'"); c != 3 {
		t.Errorf("%d checks queued, want 3", c)
	}

	// Once its checks are done, the next click starts a new run.
	e.pool.Exec(ctx, `UPDATE jobs SET status = 'done' WHERE kind = 'check_source'`)
	id, ok, err := e.p.RunNow(ctx)
	if err != nil || !ok || id == ids[0] {
		t.Errorf("after the first run: run %d, started %v, %v", id, ok, err)
	}
}

func TestSeeds(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	w := &queue.Worker{Queue: e.p.Queue, Handlers: e.p.Handlers(), PollInterval: 10 * time.Millisecond}
	inputs := []string{
		"https://www.linkedin.com/events/123",
		"Saw this: https://www.meetup.com/de-de/beispiel-group/events/99/ looks good",
		"Lea Beispiel from Beispiel Robotics",
		"www.example.org/programm?filter=koeln",
	}
	for _, in := range inputs {
		var id int64
		e.pool.QueryRow(ctx, `INSERT INTO seeds (input) VALUES ($1) RETURNING id`, in).Scan(&id)
		if err := e.p.EnqueueSeed(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.RunUntilIdle(ctx); err != nil {
		t.Fatal(err)
	}
	results := map[string]string{}
	rows, _ := e.pool.Query(ctx, `SELECT input, result || CASE WHEN processed_at IS NULL THEN ' (open)' ELSE '' END FROM seeds`)
	for rows.Next() {
		var in, r string
		rows.Scan(&in, &r)
		results[in] = r
	}
	rows.Close()
	if r := results[inputs[0]]; !strings.HasPrefix(r, "LinkedIn and Instagram links cannot be followed") {
		t.Errorf("LinkedIn seed: %q", r)
	}
	if r := results[inputs[1]]; r != "Added as a candidate source: https://www.meetup.com/beispiel-group/" {
		t.Errorf("Meetup seed: %q", r)
	}
	if r := results[inputs[2]]; !strings.HasSuffix(r, "(open)") {
		t.Errorf("a name should stay open until name search exists: %q", r)
	}
	if r := results[inputs[3]]; r != "Added as a candidate source: https://www.example.org/programm?filter=koeln" {
		t.Errorf("website seed: %q", r)
	}
	if n := e.count(t, "sources WHERE discovered_from_seed_id IS NOT NULL"); n != 2 {
		t.Errorf("%d sources from seeds, want 2", n)
	}
}

const jsShellWithEvents = `<html><head></head><body><div id="root"></div>
<noscript>You need to enable JavaScript to run this app.</noscript>
<script>
var ld = document.createElement('script');
ld.type = 'application/ld+json';
ld.text = JSON.stringify({"@context":"https://schema.org","@type":"Event","name":"Offline Evening Ehrenfeld","startDate":"2026-10-01T19:00:00+02:00","url":"https://example.org/offline","location":{"@type":"Place","name":"Café Beispiel","address":{"@type":"PostalAddress","addressLocality":"Köln"}}});
document.head.appendChild(ld);
document.getElementById('root').textContent = 'Offline Evening Ehrenfeld';
</script></body></html>`

func TestJavaScriptSourceSwitchesToBrowser(t *testing.T) {
	exec := fetch.FindChromium()
	if exec == "" {
		t.Skip("no Chromium")
	}
	e := setup(t, "")
	b := fetch.NewBrowser(fetch.BrowserOptions{ExecPath: exec})
	defer b.Close()
	e.p.Fetcher = fetch.New(fetch.Options{Browser: b})
	e.site.set("/js", jsShellWithEvents)
	e.site.set("/plain", ldPage(ldEvent("Rheinland Pitch #129", "2026-09-28T18:00:00+02:00", "Köln", "")))
	js, plain := e.addSource(t, "/js", "active"), e.addSource(t, "/plain", "active")

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	for _, id := range []int64{js, plain} {
		if err := e.p.CheckSource(ctx, id, 0); err != nil {
			t.Fatal(err)
		}
	}
	if m := e.one(t, `SELECT fetch_mode FROM sources WHERE id = $1`, js); m != "browser" {
		t.Errorf("JavaScript source mode %v, want browser", m)
	}
	if m := e.one(t, `SELECT fetch_mode FROM sources WHERE id = $1`, plain); m != "http" {
		t.Errorf("plain source mode %v, want http", m)
	}
	if n := e.count(t, "events WHERE title = 'Offline Evening Ehrenfeld' AND fit = 'kept'"); n != 1 {
		t.Error("the event rendered by JavaScript was not stored")
	}
	if n := e.count(t, "fetches WHERE mode = 'browser' AND screenshot IS NOT NULL"); n != 1 {
		t.Error("the browser fetch was not stored with a screenshot")
	}
}

// make people without addresses takes its pages from what runs found.
func TestEventPagesPicksOnePagePerSource(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	a := e.addSource(t, "/a", "active")
	b := e.addSource(t, "/b", "active")
	c := e.addSource(t, "/c", "active")
	add := func(source int64, url, fit, format string, start time.Time) {
		t.Helper()
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO events (title, starts_at, canonical_url, fit, format) VALUES ('Beispiel', $1, $2, $3, $4) RETURNING id`,
			start, url, fit, format).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO sightings (source_id, checked_at, event_id, is_new) VALUES ($1, $2, $3, true)`, source, now, id); err != nil {
			t.Fatal(err)
		}
	}
	day := 24 * time.Hour
	add(a, "https://example.org/a/later", "kept", "in_person", now.Add(5*day))
	add(a, "https://example.org/a/next", "kept", "in_person", now.Add(2*day))
	add(a, "https://example.org/a/past", "kept", "in_person", now.Add(-2*day))
	add(b, e.site.srv.URL+"/b", "kept", "in_person", now.Add(1*day))
	add(b, "https://example.org/b/dropped", "dropped", "in_person", now.Add(1*day))
	add(b, "https://example.org/b/event", "kept", "hybrid", now.Add(3*day))
	add(c, "https://example.org/c/online", "kept", "online", now.Add(1*day))
	add(c, "https://example.org/c/feed.ics", "kept", "in_person", now.Add(1*day))
	add(c, "", "kept", "in_person", now.Add(1*day))

	got, err := pipeline.EventPages(ctx, e.pool, 5, now)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.org/a/next https://example.org/b/event"
	if strings.Join(got, " ") != want {
		t.Errorf("pages %v, want %s", got, want)
	}
	if got, _ := pipeline.EventPages(ctx, e.pool, 1, now); len(got) != 1 {
		t.Errorf("%d pages with a limit of 1", len(got))
	}
}

// fakeReader stands in for the local model. It names every invented person
// whose name is on the page and quotes the line they are on.
type fakeReader struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeReader) People(_ context.Context, _, text string) (llm.PeopleResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return llm.PeopleResult{}, f.err
	}
	var res llm.PeopleResult
	for _, line := range strings.Split(text, "\n") {
		for _, name := range []string{"Lena Musterfrau", "Karl Kontrolle"} {
			if strings.Contains(line, name) {
				p := llm.Person{Name: name, Role: "speaker", Affiliation: "Beispiel GmbH", Evidence: strings.TrimSpace(line)}
				if name == "Lena Musterfrau" {
					p.Founder, p.Builds, p.FounderEvidence = true, "Backstube Muster", "hat die Backstube Muster gegründet"
				}
				res.People = append(res.People, p)
			}
		}
	}
	return res, nil
}

func eventPageSite(t *testing.T, e env) {
	t.Helper()
	ld := func(name, path string) string {
		return fmt.Sprintf(`{"@context":"https://schema.org","@type":"Event","name":%q,"startDate":"2026-10-02T18:30:00+02:00",
			"url":%q,"location":{"@type":"Place","name":"Startplatz","address":{"@type":"PostalAddress","addressLocality":"Köln"}}}`,
			name, e.site.srv.URL+path)
	}
	// A page of its own, without the Meetup link ldPage carries: a run
	// would add that as a source and check it on the live website.
	e.site.set("/events", `<html><head><script type="application/ld+json">[`+ld("Founders Talk Köln", "/e/talk")+`,`+
		ld("Pitch Abend Köln", "/e/pitch")+`]</script></head><body><h1>Events</h1></body></html>`)
	e.site.set("/e/talk", `<html><body><h1>Founders Talk</h1><p>Diesmal erzählt Lena Musterfrau von der Beispiel GmbH.</p></body></html>`)
	e.site.set("/e/pitch", `<html><body><h1>Pitch Abend</h1><p>Durch den Abend führt Karl Kontrolle.</p></body></html>`)
}

func runOnce(t *testing.T, e env) int64 {
	t.Helper()
	return runWith(t, e, 4)
}

func runWith(t *testing.T, e env, workers int) int64 {
	t.Helper()
	ctx := t.Context()
	run, err := e.p.StartRun(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.p.EnqueueDue(ctx, run); err != nil {
		t.Fatal(err)
	}
	// Several workers at once, so reads and checks overlap.
	w := &queue.Worker{Queue: e.p.Queue, Handlers: e.p.Handlers(), Concurrency: workers, PollInterval: 10 * time.Millisecond}
	if err := w.RunUntilIdle(ctx); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRunsReadEventPagesForPeople(t *testing.T) {
	e := setup(t, "")
	reader := &fakeReader{}
	e.p.Reader = reader
	eventPageSite(t, e)
	e.addSource(t, "/events", "active")

	run := runOnce(t, e)
	if c := e.count(t, "events WHERE people_read_at IS NOT NULL"); c != 2 {
		t.Fatalf("%d events read, want 2", c)
	}
	if c := e.count(t, "event_reads WHERE run_id = $1 AND error = ''", run); c != 2 {
		t.Errorf("%d reads recorded for the run", c)
	}
	if got := e.one(t, `SELECT a.evidence FROM appearances a JOIN people p ON p.id = a.person_id WHERE p.full_name = 'Lena Musterfrau'`); got != "Diesmal erzählt Lena Musterfrau von der Beispiel GmbH." {
		t.Errorf("evidence %q", got)
	}
	if c := e.count(t, "sightings WHERE person_id IS NOT NULL"); c != 2 {
		t.Errorf("%d person sightings, want one per person with the source", c)
	}
	if c := e.count(t, "affiliations af JOIN organisations o ON o.id = af.organisation_id WHERE o.name = 'Beispiel GmbH'"); c != 2 {
		t.Errorf("%d affiliations with Beispiel GmbH", c)
	}
	// A founder is marked, with the passage and what they build. Someone
	// else on stage is not.
	if got := e.one(t, `SELECT fit || ': ' || fit_evidence FROM people WHERE full_name = 'Lena Musterfrau'`); got != "founder: hat die Backstube Muster gegründet" {
		t.Errorf("Lena: %v", got)
	}
	if got := e.one(t, `SELECT fit FROM people WHERE full_name = 'Karl Kontrolle'`); got != "other" {
		t.Errorf("Karl: %v", got)
	}
	if c := e.count(t, "affiliations af JOIN organisations o ON o.id = af.organisation_id WHERE o.name = 'Backstube Muster' AND af.role = 'founder'"); c != 1 {
		t.Errorf("%d founder affiliations with Backstube Muster", c)
	}

	// A page is read once. The next run checks the source again but reads
	// nothing new.
	e.pool.Exec(t.Context(), `UPDATE sources SET next_check_at = NULL`)
	runOnce(t, e)
	if reader.calls != 2 {
		t.Errorf("the model was asked %d times, want 2", reader.calls)
	}
	if c := e.count(t, "people"); c != 2 {
		t.Errorf("%d people after two runs", c)
	}
}

func TestReadsWaitWhenNoModelAnswers(t *testing.T) {
	e := setup(t, "")
	e.p.Reader = &fakeReader{err: fmt.Errorf("%w (dial tcp: refused)", llm.ErrUnreachable)}
	eventPageSite(t, e)
	e.addSource(t, "/events", "active")
	runOnce(t, e)
	if c := e.count(t, "events WHERE people_read_at IS NULL"); c != 2 {
		t.Errorf("%d events still unread, want 2 for a later run", c)
	}
	if c := e.count(t, "jobs WHERE kind = 'read_event' AND status = 'failed'"); c != 0 {
		t.Errorf("%d reads failed, a missing model is not a failure", c)
	}
	if c := e.count(t, "event_reads WHERE error <> ''"); c == 0 {
		t.Error("the failed read is not recorded")
	}
}

// After the model fails, the run stops asking it, instead of failing page
// by page and retrying each.
func TestReadsPauseAfterTheModelFails(t *testing.T) {
	e := setup(t, "")
	reader := &fakeReader{err: fmt.Errorf("%w, it answered 500: Compute error.", llm.ErrFailed)}
	e.p.Reader = reader
	eventPageSite(t, e)
	e.addSource(t, "/events", "active")
	runWith(t, e, 1)
	if reader.calls != 1 {
		t.Errorf("the model was asked %d times after it failed, want 1", reader.calls)
	}
	if c := e.count(t, "events WHERE people_read_at IS NULL"); c != 2 {
		t.Errorf("%d events wait for a later run, want 2", c)
	}
	if c := e.count(t, "jobs WHERE kind = 'read_event' AND status = 'failed'"); c != 0 {
		t.Errorf("%d reads failed, want none retried", c)
	}
}

func TestNoReadsWithoutAModel(t *testing.T) {
	e := setup(t, "")
	eventPageSite(t, e)
	e.addSource(t, "/events", "active")
	runOnce(t, e)
	if c := e.count(t, "jobs WHERE kind = 'read_event'"); c != 0 {
		t.Errorf("%d reads queued without a model", c)
	}
	if c := e.count(t, "events WHERE people_read_at IS NULL"); c != 2 {
		t.Errorf("%d unread events, want 2", c)
	}

	// With a model, the next run reads the events found before, although
	// their source is not due.
	e.pool.Exec(t.Context(), `UPDATE sources SET next_check_at = $1`, now.Add(7*24*time.Hour))
	e.p.Reader = &fakeReader{}
	run := runOnce(t, e)
	if c := e.count(t, "source_checks WHERE run_id = $1", run); c != 0 {
		t.Errorf("%d checks in the second run, the source was not due", c)
	}
	if c := e.count(t, "event_reads WHERE run_id = $1", run); c != 2 {
		t.Errorf("%d events read from earlier runs, want 2", c)
	}
}

func TestAPageThatRefusesTheBotIsNotAskedAgain(t *testing.T) {
	e := setup(t, "")
	reader := &fakeReader{}
	e.p.Reader = reader
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer forbidden.Close()
	e.site.set("/events", ldPage(fmt.Sprintf(`{"@context":"https://schema.org","@type":"Event","name":"Geschlossene Runde",
		"startDate":"2026-10-02T18:30:00+02:00","url":%q,
		"location":{"@type":"Place","name":"Startplatz","address":{"@type":"PostalAddress","addressLocality":"Köln"}}}`, forbidden.URL+"/e/closed")))
	e.addSource(t, "/events", "active")
	runOnce(t, e)
	if c := e.count(t, "event_reads WHERE error <> ''"); c != 1 {
		t.Errorf("%d reads with an error, want 1", c)
	}
	if c := e.count(t, "events WHERE people_read_at IS NOT NULL"); c != 1 {
		t.Errorf("the refused page must count as read, %d are", c)
	}
	if reader.calls != 0 {
		t.Errorf("the model was asked %d times about a page it never got", reader.calls)
	}
}

func TestStopRunDropsWhatIsQueued(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	for i := range 3 {
		e.addSource(t, fmt.Sprintf("/s%d", i), "active")
	}
	run, _, err := e.p.RunNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := e.p.StopRun(ctx, run)
	if err != nil || !stopped {
		t.Fatalf("stop: %v %v", stopped, err)
	}
	if c := e.count(t, "jobs WHERE status = 'queued' AND key LIKE $1", fmt.Sprintf("run:%d:%%", run)); c != 0 {
		t.Errorf("%d jobs of the stopped run still queued", c)
	}
	if again, _ := e.p.StopRun(ctx, run); again {
		t.Error("a run can only be stopped once")
	}
	// A new run can start right away.
	if _, started, err := e.p.RunNow(ctx); err != nil || !started {
		t.Errorf("a run after the stopped one: started %v, %v", started, err)
	}
}

// A run checks at most sources_per_run due sources, the longest overdue
// first, so runs stay short and the rest wait for the next one.
func TestRunsStayShort(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	var ids []int64
	for i := range 4 {
		id := e.addSource(t, fmt.Sprintf("/s%d", i), "active")
		e.pool.Exec(ctx, `UPDATE sources SET next_check_at = $2 WHERE id = $1`, id, now.Add(-time.Duration(i+1)*time.Hour))
		ids = append(ids, id)
	}
	e.pool.Exec(ctx, `UPDATE settings SET value = '2' WHERE key = 'sources_per_run'`)
	run, _ := e.p.StartRun(ctx, "manual")
	if n, err := e.p.EnqueueDue(ctx, run); err != nil || n != 2 {
		t.Fatalf("%d sources queued, %v, want 2", n, err)
	}
	// The two overdue the longest are s3 and s2.
	for _, id := range ids[2:] {
		if c := e.count(t, "jobs WHERE key = $1", fmt.Sprintf("run:%d:source:%d", run, id)); c != 1 {
			t.Errorf("source %d, among the longest overdue, is not queued", id)
		}
	}
}

// After the app stopped in the middle of a run, the run's work does not come
// back by itself when the app starts again.
func TestInterruptedRunsEndWhenTheAppStarts(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	for i := range 3 {
		e.addSource(t, fmt.Sprintf("/s%d", i), "active")
	}
	run, _, err := e.p.RunNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The app died while one check ran.
	e.pool.Exec(ctx, `UPDATE jobs SET status = 'running', locked_until = $1 WHERE id = (SELECT min(id) FROM jobs WHERE kind = 'check_source')`, now.Add(10*time.Minute))
	n, err := e.p.EndInterrupted(ctx)
	if err != nil || n != 1 {
		t.Fatalf("ended %d runs, %v, want 1", n, err)
	}
	if c := e.count(t, "jobs WHERE kind = 'check_source' AND status IN ('queued', 'running')"); c != 0 {
		t.Errorf("%d checks would come back", c)
	}
	if c := e.count(t, "runs WHERE id = $1 AND finished_at IS NOT NULL", run); c != 1 {
		t.Error("the interrupted run did not end")
	}
	if c := e.count(t, "jobs WHERE kind = 'prune' AND status = 'queued'"); c != 1 {
		t.Error("housekeeping outside a run must stay")
	}
}

// A page whose read keeps failing is given up after three tries, so it does
// not come back in every run.
func TestAPageThatKeepsFailingIsGivenUp(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	e.p.Reader = &fakeReader{}
	eventPageSite(t, e)
	e.addSource(t, "/events", "active")
	runOnce(t, e)
	e.pool.Exec(ctx, `UPDATE events SET people_read_at = NULL`)
	var ev int64
	e.pool.QueryRow(ctx, `SELECT id FROM events WHERE canonical_url LIKE '%/e/talk'`).Scan(&ev)
	for range 3 {
		e.pool.Exec(ctx, `INSERT INTO event_reads (event_id, url, error, read_at) VALUES ($1, 'x', 'the language model failed', $2)`, ev, now)
	}
	run, _ := e.p.StartRun(ctx, "manual")
	if _, err := e.p.EnqueueDue(ctx, run); err != nil {
		t.Fatal(err)
	}
	if c := e.count(t, "jobs WHERE kind = 'read_event' AND status = 'queued' AND payload->>'event_id' = $1", fmt.Sprint(ev)); c != 0 {
		t.Error("a page that failed three times is read again")
	}
	if c := e.count(t, "jobs WHERE kind = 'read_event' AND status = 'queued'"); c != 1 {
		t.Errorf("%d reads queued, want the other page", c)
	}
}

// A title makes a founder only when its role says so, never an
// organisation's name, and a CEO or managing director alone is not one.
// All organisations are invented.
func TestFounderFromTitle(t *testing.T) {
	for aff, want := range map[string][2]string{
		"Co-founder and CEO, Beispiel Robotics":     {"Beispiel Robotics", "founder"},
		"Gründerin, Backstube Muster":               {"Backstube Muster", "founder"},
		"Mitgründerin von Beispielwerk":             {"Beispielwerk", "founder"},
		"Inhaberin, Café Beispiel":                  {"Café Beispiel", "founder"},
		"CEO, Musterbank AG":                        {"Musterbank AG", "employee"},
		"Geschäftsführer, Beispielverband e.V.":     {"Beispielverband e.V", "employee"},
		"Projektleiter, Gründerzentrum Musterstadt": {"Gründerzentrum Musterstadt", "employee"},
		"Gründer-Stammtisch Musterstadt":            {"Gründer-Stammtisch Musterstadt", "employee"},
		"Founders Foundation Musterstadt":           {"Founders Foundation Musterstadt", "employee"},
		"Team Lead at Beispiel Founders Club":       {"Beispiel Founders Club", "employee"},
	} {
		org, role := pipeline.ParseAffiliation(aff)
		if org != want[0] || role != want[1] {
			t.Errorf("%q: %q, %q, want %q, %q", aff, org, role, want[0], want[1])
		}
	}
}

// A link becomes the organiser's calendar, never one of a platform's own
// pages or a single event. The organisers are invented.
func TestCalendarURL(t *testing.T) {
	for link, want := range map[string]string{
		"https://www.meetup.com/beispiel-founders-koeln/events/316565851/":    "https://www.meetup.com/beispiel-founders-koeln/",
		"https://www.meetup.com/de-DE/beispiel-founders-koeln/":               "https://www.meetup.com/beispiel-founders-koeln/",
		"https://www.meetup.com/lp/":                                          "",
		"https://www.meetup.com/find/?location=de--Koeln":                     "",
		"https://www.meetup.com/apps/":                                        "",
		"https://www.meetup.com/login/":                                       "",
		"https://www.tickettailor.com/events/beispielnightskoeln/2432028":     "https://www.tickettailor.com/events/beispielnightskoeln",
		"https://www.tickettailor.com/events/beispielnightskoeln/2432028/r/x": "https://www.tickettailor.com/events/beispielnightskoeln",
		"https://www.tickettailor.com/events/beispielnightskoeln":             "https://www.tickettailor.com/events/beispielnightskoeln",
	} {
		if got := pipeline.CalendarURL(link); got != want {
			t.Errorf("%s: %q, want %q", link, got, want)
		}
	}
}

// portfolioSite is an invented hub's portfolio of four startups, each with
// its own website and imprint. All names are invented.
func portfolioSite(t *testing.T, e env) int64 {
	t.Helper()
	e.site.set("/portfolio", `<html><body>
<header><a href="https://partner.test/">Partner</a></header>
<main><h1>Our startups</h1>
<a href="http://beispiel-robotics.test/"><img alt="Beispiel Robotics" src="a.png"></a>
<a href="http://probe-labs.test/">Probe Labs</a>
<a href="http://musterbank.test/">Musterbank</a>
<a href="http://schweigen.test/">Schweigen</a>
<a href="https://www.linkedin.com/company/beispiel">LinkedIn</a>
</main>
<footer><a href="http://sponsor.test/">Sponsor</a></footer></body></html>`)
	e.site.set("http://beispiel-robotics.test/", `<html><body><h1>Robots</h1><header><a href="/team">Team</a></header>
<footer><a href="/rechtliches">Impressum</a><a href="https://www.linkedin.com/company/beispiel-robotics">LinkedIn</a></footer></body></html>`)
	// The team page links profiles. Only those with the founder's name count.
	e.site.set("http://beispiel-robotics.test/team", `<main><h3>Lena Musterfrau</h3><a href="https://www.linkedin.com/in/lena-musterfrau-4b2a1/">in</a>
<h3>Tom Testmann</h3><a href="https://de.linkedin.com/in/ACoAAB12xyz">in</a><h3>Karl Kontrolle</h3><a href="https://x.com/kkontrolle">x</a></main>`)
	e.site.set("http://beispiel-robotics.test/rechtliches", `<html><body><h1>Impressum</h1>
<p>Beispiel Robotics GmbH<br>Musterstraße 1<br>50667 Köln</p>
<p>Geschäftsführer: Lena Musterfrau, Tom Testmann</p>
<p>E-Mail: hallo@beispiel-robotics.test, Telefon: +49 221 000000</p></body></html>`)
	// No link to the imprint. It sits where most imprints sit.
	e.site.set("http://probe-labs.test/", `<html><body><h1>Probe Labs</h1></body></html>`)
	e.site.set("http://probe-labs.test/impressum", `<html><body>
<p>Probe Labs UG (haftungsbeschränkt), 44137 Dortmund</p>
<p>Vertreten durch die Geschäftsführerin Mara Beispielfrau</p></body></html>`)
	e.site.set("http://musterbank.test/", `<html><body><a href="/impressum">Impressum</a></body></html>`)
	e.site.set("http://musterbank.test/impressum", `<html><body><p>Musterbank AG</p><p>Vorstand: Erika Erfunden, Max Mustermann</p></body></html>`)
	// schweigen.test has no pages at all.
	var id int64
	if err := e.pool.QueryRow(t.Context(), `INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel Hub', 'portfolio', $1, 'candidate') RETURNING id`,
		e.site.srv.URL+"/portfolio").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPortfolioLeadsToWhoRunsEachStartup(t *testing.T) {
	e := setup(t, "")
	src := portfolioSite(t, e)
	run := runOnce(t, e)

	if got := e.one(t, `SELECT status || ' ' || next_check_at::date::text FROM sources WHERE id = $1`, src); got != "active 2026-10-09" {
		t.Errorf("portfolio after its check: %v", got)
	}
	if got := e.one(t, `SELECT startups_found || ' found, ' || startups_new || ' new' FROM source_checks WHERE source_id = $1`, src); got != "4 found, 4 new" {
		t.Errorf("check: %v", got)
	}
	if c := e.count(t, "startup_lookups WHERE run_id = $1", run); c != 4 {
		t.Errorf("%d lookups in the run, want 4", c)
	}
	// The managing directors of the two young companies are founders, with
	// the imprint as the reason.
	rows, err := e.pool.Query(t.Context(), `
		SELECT p.full_name || ', ' || p.city || ': ' || p.fit_evidence FROM people p
		JOIN affiliations af ON af.person_id = p.id AND af.role = 'founder' WHERE p.fit = 'founder' ORDER BY p.full_name`)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := pgx.CollectRows(rows, pgx.RowTo[string])
	want := []string{
		"Lena Musterfrau, Köln: Managing director of Beispiel Robotics GmbH, by its imprint. In the portfolio of Beispiel Hub",
		"Mara Beispielfrau, Dortmund: Managing director of Probe Labs UG (haftungsbeschränkt), by its imprint. In the portfolio of Beispiel Hub",
		"Tom Testmann, Köln: Managing director of Beispiel Robotics GmbH, by its imprint. In the portfolio of Beispiel Hub",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("founders:\n%s", strings.Join(got, "\n"))
	}
	// A bank's board is nobody Tim looks for, and a site that does not
	// answer is tried again by a later run.
	if c := e.count(t, "people"); c != 3 {
		t.Errorf("%d people, want 3", c)
	}
	if got := e.one(t, `SELECT l.note FROM startup_lookups l JOIN organisations o ON o.id = l.organisation_id WHERE o.website LIKE '%musterbank%'`); got != "not a young company (AG)" {
		t.Errorf("the bank's lookup says %q", got)
	}
	if c := e.count(t, "organisations WHERE website LIKE '%schweigen%' AND looked_up_at IS NULL"); c != 1 {
		t.Error("a startup whose site did not answer is not left for a later run")
	}
	if got := e.one(t, `SELECT string_agg(p.full_name || ' ' || pr.url || ' ' || pr.review || ' ' || pr.found_via, ', ') FROM profiles pr JOIN people p ON p.id = pr.person_id`); got != "Lena Musterfrau https://www.linkedin.com/in/lena-musterfrau-4b2a1 open website of beispiel-robotics.test" {
		t.Errorf("profiles: %v", got)
	}
	if c := e.count(t, "people WHERE headline LIKE '%@%' OR headline ~ '[0-9]{4}' OR full_name ~ '[0-9@]'"); c != 0 {
		t.Error("contact details were stored")
	}

	// The next run looks up nothing twice, and only the silent site again.
	runOnce(t, e)
	if c := e.count(t, "startup_lookups"); c != 5 {
		t.Errorf("%d lookups after two runs, want 5", c)
	}
	if c := e.count(t, "people"); c != 3 {
		t.Errorf("%d people after two runs", c)
	}
}

// Many portfolios link to a page of their own about each startup, over
// several pages. All names are invented.
func TestPortfolioWithPagesAboutEachStartup(t *testing.T) {
	e := setup(t, "")
	e.site.set("/our-startups", `<main><a href="/startups/beispiel-robotics">Beispiel Robotics</a>
<a href="/startups/probe-labs">Probe Labs</a><a href="/startups/stumm">Stumm</a><a href="/our-startups/p2">2</a></main>`)
	e.site.set("/our-startups/p2", `<main><a href="/startups/muster-health">Muster Health</a><a href="/startups/probe-labs">Probe Labs</a>
<a href="/startups/stumm">Stumm</a><a href="/our-startups">1</a></main>`)
	e.site.set("/startups/beispiel-robotics", `<main><p>Robots for bakeries.</p><a href="http://beispiel-robotics.test/">Website</a></main>`)
	e.site.set("/startups/probe-labs", `<main><a href="http://probe-labs.test/">Go to Probe Labs</a></main>`)
	e.site.set("/startups/muster-health", `<main><a href="http://muster-health.test/de">Website</a></main>`)
	e.site.set("/startups/stumm", `<main><p>No link to anywhere.</p></main>`)
	e.site.set("http://beispiel-robotics.test/", `<footer><a href="/impressum">Impressum</a></footer>`)
	e.site.set("http://beispiel-robotics.test/impressum", `<p>Beispiel Robotics GmbH</p><p>50667 Köln</p><p>Geschäftsführerin: Lena Musterfrau</p>`)
	e.site.set("http://probe-labs.test/", `<h1>Probe</h1>`)
	e.site.set("http://probe-labs.test/impressum", `<p>Probe Labs UG (haftungsbeschränkt)</p><p>Geschäftsführer: Tom Testmann</p>`)
	e.site.set("http://muster-health.test/", `<a href="/legal">Imprint</a>`)
	e.site.set("http://muster-health.test/legal", `<p>Muster Health GmbH, 40210 Düsseldorf</p><p>Managing Director: Mara Beispielfrau</p>`)
	var src int64
	if err := e.pool.QueryRow(t.Context(), `INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel Hub', 'portfolio', $1, 'candidate') RETURNING id`,
		e.site.srv.URL+"/our-startups").Scan(&src); err != nil {
		t.Fatal(err)
	}
	runOnce(t, e)
	if got := e.one(t, `SELECT startups_found || ' found' FROM source_checks WHERE source_id = $1`, src); got != "4 found" {
		t.Errorf("check over two pages: %v", got)
	}
	rows, err := e.pool.Query(t.Context(), `SELECT o.name || ', ' || o.website FROM organisations o ORDER BY o.name`)
	if err != nil {
		t.Fatal(err)
	}
	orgs, _ := pgx.CollectRows(rows, pgx.RowTo[string])
	want := []string{
		"Beispiel Robotics GmbH, http://beispiel-robotics.test/",
		"Muster Health GmbH, http://muster-health.test/",
		"Probe Labs UG (haftungsbeschränkt), http://probe-labs.test/",
		"Stumm, ",
	}
	if strings.Join(orgs, "\n") != strings.Join(want, "\n") {
		t.Errorf("startups:\n%s", strings.Join(orgs, "\n"))
	}
	if c := e.count(t, "people WHERE fit = 'founder'"); c != 3 {
		t.Errorf("%d founders, want 3", c)
	}
	if got := e.one(t, `SELECT l.note FROM startup_lookups l JOIN organisations o ON o.id = l.organisation_id WHERE o.name = 'Stumm'`); got != "no website on its portfolio page" {
		t.Errorf("the startup without a website: %q", got)
	}
}
