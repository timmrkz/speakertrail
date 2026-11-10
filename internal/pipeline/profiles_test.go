package pipeline_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/search"
)

// lastWeek is a date of activity within the last six months of now.
var lastWeek = now.AddDate(0, 0, -7)

// Invented people, as a profile search for "Personal Trainer Köln" finds
// them in Exa's index: some who fit, and each reason someone does not.
func newFakePeople() *fakePosts {
	in := func(handle string) string { return "https://www.linkedin.com/in/" + handle }
	return &fakePosts{people: []search.Profile{
		{URL: in("lea-probe"), Name: "Lea Probe", Headline: "Personal Trainerin | Selbstständig", Location: "Cologne, North Rhine-Westphalia, Germany",
			LastActive: lastWeek, Career: []search.Step{
				{Title: "Personal Trainerin", Org: "Selbstständig", From: 2023},
				{Title: "Werkstudentin", Org: "Muster Fitness", From: 2020, To: 2023},
				{Education: true, Title: "Bachelor of Science, Sportwissenschaft", Org: "Beispiel Sporthochschule", From: 2019, To: 2023},
			}},
		{URL: in("tom-muster"), Name: "Tom Muster", Headline: "Gründer @ Probe Gym", Location: "Düsseldorf, North Rhine-Westphalia, Germany",
			LastActive: now.AddDate(0, -2, 0), Career: []search.Step{
				{Title: "Founder & Managing Director", Org: "Probe Gym GmbH", From: 2016},
				{Title: "Trainer", Org: "Muster Club", From: 2008, To: 2016},
			}},
		// Not in NRW.
		{URL: in("ida-beispiel"), Name: "Ida Beispiel", Headline: "Personal Trainerin", Location: "Berlin, Berlin, Germany",
			LastActive: lastWeek, Career: []search.Step{{Title: "Inhaberin", Org: "Beispiel Studio", From: 2022}}},
		// Quiet on LinkedIn for a year.
		{URL: in("ole-probe"), Name: "Ole Probe", Headline: "Personal Trainer", Location: "Bonn, North Rhine-Westphalia, Germany",
			LastActive: now.AddDate(-1, 0, 0), Career: []search.Step{{Title: "Inhaber", Org: "Probe Fitness", From: 2015}}},
		// A management consultant who founded his firm.
		{URL: in("max-beispiel"), Name: "Max Beispiel", Headline: "Management Consultant | Strategy & Transformation", Location: "Köln",
			LastActive: lastWeek, Career: []search.Step{{Title: "Founder", Org: "Beispiel Consulting", From: 2019}}},
		// A founder in online marketing, left out by one of Tim's
		// exclusions.
		{URL: in("nina-probe"), Name: "Nina Probe", Headline: "Gründerin | Online-Marketing für Coaches", Location: "Köln",
			LastActive: lastWeek, Career: []search.Step{{Title: "Gründerin", Org: "Probe Sichtbar", From: 2023}}},
		// A trainer employed by a club.
		{URL: in("mia-muster"), Name: "Mia Muster", Headline: "Athletiktrainerin", Location: "Essen, North Rhine-Westphalia, Germany",
			LastActive: lastWeek, Career: []search.Step{{Title: "Athletiktrainerin", Org: "Probe Sportverein", From: 2021}}},
	}}
}

// A run takes only profile searches. Of the people they find, only those
// in NRW, active on LinkedIn lately and running their own business are
// kept, with their profile confirmed, their likely age and the roles they
// hold now. Nothing is stored of anyone else. Other sources wait.
func TestProfileSearchesKeepOnlyPossibleGuests(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	if _, err := e.pool.Exec(ctx, `UPDATE settings SET value = 'true' WHERE key = 'profile_searches_only'`); err != nil {
		t.Fatal(err)
	}
	fp := newFakePeople()
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fp}, Now: func() time.Time { return now }}
	var src, events int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ('Personal Trainer Köln', 'profile_search', 'Personal Trainer Köln', 'candidate') RETURNING id`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel events', 'listing', $1, 'active') RETURNING id`, e.site.srv.URL+"/events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	run := runOnce(t, e)

	if strings.Join(fp.queries, "|") != "people Personal Trainer Köln, 25" {
		t.Errorf("searches %q", fp.queries)
	}
	if c := e.count(t, "source_checks WHERE source_id = $1", events); c != 0 {
		t.Errorf("the paused event source was checked %d times", c)
	}
	if got := e.one(t, `SELECT people_found || ' ' || people_new || ' ' || mode FROM source_checks WHERE source_id = $1 AND run_id = $2`, src, run); got != "2 2 search" {
		t.Errorf("the profile search's check: %v", got)
	}
	got := e.one(t, `SELECT string_agg(p.full_name || ', ' || p.city || ': ' || pr.review || ', ' || COALESCE(pr.born_about::text, '-') || ' ' || pr.age_reason || ', ' || pr.current_roles || ', ' || to_char(pr.last_active_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), ' | ' ORDER BY p.full_name)
		FROM people p JOIN profiles pr ON pr.person_id = p.id`)
	want := "Lea Probe, Köln: confirmed, 2000 Bachelor from 2019, Personal Trainerin, Selbstständig, 2026-09-18" +
		" | Tom Muster, Düsseldorf: confirmed, 1986 First job from 2008, Founder & Managing Director, Probe Gym GmbH, 2026-07-25"
	if got != want {
		t.Errorf("people kept\n%v\nwant\n%v", got, want)
	}
	if got := e.one(t, `SELECT string_agg(p.full_name || ': ' || s.signal, ', ' ORDER BY p.full_name, s.signal) FROM person_signals s JOIN people p ON p.id = s.person_id`); got != "Lea Probe: active_on_linkedin, Lea Probe: owner_operator, Lea Probe: works_with_people, Lea Probe: young, Tom Muster: active_on_linkedin, Tom Muster: owner_operator, Tom Muster: works_with_people" {
		t.Errorf("signals: %v", got)
	}
	if got := e.one(t, `SELECT status || ' ' || to_char(next_check_at - last_checked_at, 'DD') FROM sources WHERE id = $1`, src); got != "active 29" {
		t.Errorf("the profile search after it found people: %v", got)
	}
	// Found again, nobody is doubled, and a search that is not due waits.
	runOnce(t, e)
	if len(fp.queries) != 1 {
		t.Errorf("searches after a second run %q", fp.queries)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE sources SET next_check_at = NULL WHERE id = $1`, src); err != nil {
		t.Fatal(err)
	}
	runOnce(t, e)
	if c := e.count(t, "people"); c != 2 {
		t.Errorf("%d people after the third run, want 2", c)
	}
}

// Profile searches side by side that find the same person keep one.
func TestProfileSearchesSideBySideFindOnePerson(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	fp := newFakePeople()
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fp}, Now: func() time.Time { return now }}
	var ids []int64
	for _, q := range []string{"Personal Trainer Köln", "Personal Trainer Düsseldorf", "Gen Z Unternehmer NRW", "Fitnessstudio Inhaber Köln"} {
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ($1, 'profile_search', $1, 'candidate') RETURNING id`, q).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			if err := e.p.CheckSource(ctx, id, 0); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := e.one(t, `SELECT string_agg(full_name, ', ' ORDER BY full_name) FROM people`); got != "Lea Probe, Tom Muster" {
		t.Errorf("people: %v", got)
	}
	if c := e.count(t, "profiles"); c != 2 {
		t.Errorf("%d profiles, want 2", c)
	}
}

// Without Exa a profile search waits, and the run asks nothing.
func TestProfileSearchesWaitWithoutExa(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	if _, err := e.pool.Exec(ctx, `UPDATE settings SET value = 'true' WHERE key = 'profile_searches_only'`); err != nil {
		t.Fatal(err)
	}
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{&fakeSearch{}}, Now: func() time.Time { return now }}
	var src int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ('Personal Trainer Köln', 'profile_search', 'Personal Trainer Köln', 'candidate') RETURNING id`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	runOnce(t, e)
	if got := e.one(t, `SELECT status || ' ' || checks || ' ' || (next_check_at IS NULL) FROM sources WHERE id = $1`, src); got != "candidate 0 true" {
		t.Errorf("the profile search without Exa: %v", got)
	}
}

// A run takes the due profile searches best first: one that never ran,
// then the one whose people Tim connected with, before one whose people he
// never connected with.
func TestRunsTakeTheBestSearchesFirst(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	for _, sql := range []string{
		`UPDATE settings SET value = 'true' WHERE key = 'profile_searches_only'`,
		`UPDATE settings SET value = '2' WHERE key = 'profile_searches_per_run'`,
	} {
		if _, err := e.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	fp := newFakePeople()
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fp}, Now: func() time.Time { return now }}
	ids := map[string]int64{}
	for _, q := range []string{"Yoga Studio Inhaberin Köln", "Life Coach Bonn", "student founder NRW"} {
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ($1, 'profile_search', $1, 'candidate') RETURNING id`, q).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[q] = id
	}
	// Both ran before and found one person each. Tim connected with the
	// one Life Coach Bonn found.
	for i, q := range []string{"Yoga Studio Inhaberin Köln", "Life Coach Bonn"} {
		var person int64
		status := []string{"new", "connected"}[i]
		if err := e.pool.QueryRow(ctx, `INSERT INTO people (full_name, normalised_name, podcast_status) VALUES ($1, lower($1), $2) RETURNING id`, "Probe "+q, status).Scan(&person); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO sightings (source_id, person_id, checked_at, is_new) VALUES ($1, $2, $3, true)`, ids[q], person, now.AddDate(0, -2, 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE sources SET last_checked_at = $2, next_check_at = $2 WHERE id = $1`, ids[q], now.AddDate(0, -1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	runOnce(t, e)
	got := strings.Join(fp.queries, "|")
	if !strings.Contains(got, "student founder NRW") || !strings.Contains(got, "Life Coach Bonn") || strings.Contains(got, "Yoga") {
		t.Errorf("searches %q, want the new one and the one that led to a connection", got)
	}
}

// An age the headline gives beats the guess from the years on the profile.
func TestProfileSearchesTakeTheAgeTheHeadlineGives(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	fp := &fakePosts{people: []search.Profile{{
		URL: "https://www.linkedin.com/in/ben-beispiel", Name: "Ben Beispiel", Headline: "19 year old student and founder of Beispiel Labs",
		Location: "Köln", LastActive: lastWeek, Career: []search.Step{
			{Title: "Founder", Org: "Beispiel Labs", From: 2025},
			{Education: true, Title: "Bachelor of Science", Org: "Probe Universität", From: 2020},
		}}}}
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fp}, Now: func() time.Time { return now }}
	var src int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ('student founder Köln', 'profile_search', 'student founder Köln', 'candidate') RETURNING id`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if err := e.p.CheckSource(ctx, src, 0); err != nil {
		t.Fatal(err)
	}
	if got := e.one(t, `SELECT born_about || ' ' || age_reason FROM profiles`); got != "2007 Headline says 19 in 2026" {
		t.Errorf("age: %v", got)
	}
}
