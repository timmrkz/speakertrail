package pipeline_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/search"
)

// fakePosts finds the same LinkedIn posts for every query, and knows the
// profiles of some of their authors, like Exa's index.
type fakePosts struct {
	mu       sync.Mutex
	queries  []string
	lookups  [][]string
	posts    []search.Post
	profiles map[string]search.Profile
}

func (f *fakePosts) Name() string { return "exa" }

func (f *fakePosts) Search(context.Context, string) ([]search.Result, error) { return nil, nil }

func (f *fakePosts) Posts(_ context.Context, q string, since time.Time) ([]search.Post, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q+" since "+since.Format("2006-01-02"))
	return f.posts, nil
}

func (f *fakePosts) Profiles(_ context.Context, urls []string) ([]search.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, urls)
	var out []search.Profile
	for _, u := range urls {
		if p, ok := f.profiles[u]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func post(author, slug, title string, day int) search.Post {
	return search.Post{
		URL:   "https://www.linkedin.com/posts/" + author + "_" + slug + "-activity-1",
		Title: title, Published: time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC),
		Author: "https://www.linkedin.com/in/" + author,
	}
}

// Invented authors: one in Köln, one in the Ruhr already known from a
// website, one in Berlin, and a studio posting as a company.
func newFakePosts() *fakePosts {
	return &fakePosts{
		posts: []search.Post{
			post("erika-beispiel", "ein-jahr", "Ein Jahr selbstständig", 3),
			post("erika-beispiel", "neues-studio", "Unser Studio ist eröffnet", 14),
			post("max-probe", "gekuendigt", "Ich habe gekündigt", 10),
			post("lena-muster", "coach", "Mein Weg zum Coach", 12),
			post("probe-studio", "angebot", "Neue Kurse ab Oktober", 20),
		},
		profiles: map[string]search.Profile{
			"https://www.linkedin.com/in/erika-beispiel": {URL: "https://www.linkedin.com/in/erika-beispiel", Name: "Erika Beispiel",
				Headline: "Yogalehrerin und Inhaberin", Location: "Cologne, North Rhine-Westphalia, Germany"},
			"https://www.linkedin.com/in/max-probe": {URL: "https://www.linkedin.com/in/max-probe", Name: "Max Probe", Location: "Ruhr Region"},
			"https://www.linkedin.com/in/lena-muster": {URL: "https://www.linkedin.com/in/lena-muster", Name: "Lena Muster",
				Headline: "Coach", Location: "Berlin, Berlin, Germany"},
		},
	}
}

// A post search finds people who write on LinkedIn, and keeps only those
// who live in NRW, each with the post that brought them. The engine asks
// only the provider, never LinkedIn.
func TestPostSearchesFindActivePeopleInNRW(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	fp := newFakePosts()
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{&fakeSearch{}, fp}, Now: func() time.Time { return now }}
	// Max Probe is known already, through his gym's website.
	var max int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO people (full_name, normalised_name, city, fit, fit_evidence) VALUES ('Max Probe', 'max probe', 'Bochum', 'founder', 'Owner of Probe Kampfsport, by its imprint') RETURNING id`).Scan(&max); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO profiles (person_id, platform, url, handle, review) VALUES ($1, 'linkedin', 'https://www.linkedin.com/in/max-probe', 'max-probe', 'open')`, max); err != nil {
		t.Fatal(err)
	}
	var src int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ('Ein Jahr selbstständig #köln', 'post_search', 'Ein Jahr selbstständig #köln', 'candidate') RETURNING id`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	run := runOnce(t, e)

	if strings.Join(fp.queries, "|") != "Ein Jahr selbstständig #köln since 2026-06-27" {
		t.Errorf("post searches %q", fp.queries)
	}
	// One lookup for all four authors, each once.
	if len(fp.lookups) != 1 || len(fp.lookups[0]) != 4 {
		t.Errorf("lookups %q", fp.lookups)
	}
	if got := e.one(t, `SELECT people_found || ' ' || people_new || ' ' || mode FROM source_checks WHERE source_id = $1 AND run_id = $2`, src, run); got != "2 1 search" {
		t.Errorf("the post search's check: %v", got)
	}
	got := e.one(t, `SELECT string_agg(full_name || ', ' || city || ', ' || headline || ': ' || fit_evidence, ' | ' ORDER BY full_name) FROM people`)
	want := "Erika Beispiel, Köln, Yogalehrerin und Inhaberin: Wrote on LinkedIn on 14 September 2026: “Unser Studio ist eröffnet”. Found by the post search Ein Jahr selbstständig #köln" +
		" | Max Probe, Bochum, : Owner of Probe Kampfsport, by its imprint"
	if got != want {
		t.Errorf("people\n%v\nwant\n%v", got, want)
	}
	// The profile they were found by is theirs for certain.
	if got := e.one(t, `SELECT string_agg(handle || ' ' || review, ', ' ORDER BY handle) FROM profiles`); got != "erika-beispiel confirmed, max-probe open" {
		t.Errorf("profiles: %v", got)
	}
	if got := e.one(t, `SELECT string_agg(p.full_name || ': ' || pp.title || ' ' || to_char(pp.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), ' | ' ORDER BY p.full_name) FROM person_posts pp JOIN people p ON p.id = pp.person_id`); got != "Erika Beispiel: Unser Studio ist eröffnet 2026-09-14 | Max Probe: Ich habe gekündigt 2026-09-10" {
		t.Errorf("posts kept: %v", got)
	}
	if got := e.one(t, `SELECT status || ' ' || to_char(next_check_at - last_checked_at, 'DD') FROM sources WHERE id = $1`, src); got != "active 13" {
		t.Errorf("the post search after it found people: %v", got)
	}
	if got := e.one(t, `SELECT string_agg(provider || ' ' || query || ' ' || results, ' | ' ORDER BY id) FROM search_calls`); got != "exa Ein Jahr selbstständig #köln 5 | exa 4 profiles 3" {
		t.Errorf("calls counted: %v", got)
	}
	// Found again, nobody is doubled.
	if _, err := e.pool.Exec(ctx, `UPDATE sources SET next_check_at = NULL WHERE id = $1`, src); err != nil {
		t.Fatal(err)
	}
	runOnce(t, e)
	if c := e.count(t, "people"); c != 2 {
		t.Errorf("%d people after the second run, want 2", c)
	}
}

// Two post searches run side by side and find the same author. She is one
// person.
func TestPostSearchesSideBySideFindOnePerson(t *testing.T) {
	e := setup(t, "")
	ctx := t.Context()
	fp := newFakePosts()
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fp}, Now: func() time.Time { return now }}
	var ids []int64
	for _, q := range []string{"Ein Jahr selbstständig #köln", "Neues Studio #köln", "Mein Weg #köln", "Gekündigt #ruhrgebiet"} {
		var id int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO sources (name, kind, query, status) VALUES ($1, 'post_search', $1, 'candidate') RETURNING id`, q).Scan(&id); err != nil {
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
	if got := e.one(t, `SELECT string_agg(full_name, ', ' ORDER BY full_name) FROM people`); got != "Erika Beispiel, Max Probe" {
		t.Errorf("people: %v", got)
	}
	if c := e.count(t, "person_posts"); c != 2 {
		t.Errorf("%d posts kept, want 2", c)
	}
}

// Without a provider that finds posts, a post search waits, and the
// website search provider is not asked for it.
func TestPostSearchesWaitWithoutExa(t *testing.T) {
	e := setup(t, "")
	fs := &fakeSearch{}
	e.p.Search = &search.Pool{DB: e.pool, Providers: []search.Provider{fs}, Now: func() time.Time { return now }}
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO sources (name, kind, query, status) VALUES ('Neues Studio #köln', 'post_search', 'Neues Studio #köln', 'candidate')`); err != nil {
		t.Fatal(err)
	}
	run := runOnce(t, e)
	if c := e.count(t, "source_checks WHERE run_id = $1", run); c != 0 {
		t.Errorf("%d checks without Exa", c)
	}
	if len(fs.queries) != 0 || e.count(t, "search_calls") != 0 {
		t.Error("a provider was asked for posts it cannot find")
	}
}
