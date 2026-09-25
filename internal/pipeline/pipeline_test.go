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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
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
	p := &pipeline.Pipeline{Pool: pool, Fetcher: fetch.New(fetch.Options{}), Queue: q, Now: func() time.Time { return now }}
	return env{pool: pool, p: p, site: s}
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
	// Active and probation that are due, plus 10 new candidates.
	if n != 12 {
		t.Errorf("%d sources queued, want 12", n)
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
	if c := e.count(t, "jobs WHERE kind = 'check_source'"); c != 12 {
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
