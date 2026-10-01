package search_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/search"
)

func TestAuthorOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.linkedin.com/posts/erika-beispiel_ein-jahr-selbstst%C3%A4ndig-activity-7208940214981906432-_det": "https://www.linkedin.com/in/erika-beispiel",
		"https://de.linkedin.com/posts/max-probe-123abc_ich-habe-gekuendigt-activity-1":                               "https://www.linkedin.com/in/max-probe-123abc",
		"https://www.linkedin.com/posts/lena-m%C3%BCster_neues-studio-activity-2":                                     "https://www.linkedin.com/in/lena-m%C3%BCster",
		"https://www.linkedin.com/in/erika-beispiel":                                                                  "",
		"https://www.linkedin.com/posts/ohne-unterstrich":                                                             "",
		"https://beispiel.example/posts/erika-beispiel_ein-jahr":                                                      "",
		"https://linkedin.com.example/posts/erika-beispiel_ein-jahr":                                                  "",
	} {
		if got := search.AuthorOf(in); got != want {
			t.Errorf("AuthorOf(%s) = %q, want %q", in, got, want)
		}
	}
}

// exaPostServer answers like Exa's search API limited to LinkedIn posts,
// and like its contents API for profiles. It checks that both only read
// Exa's own index, never LinkedIn live.
func exaPostServer(t *testing.T, searches, contents *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("x-api-key") != "exa-test" {
			t.Errorf("exa request %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/search":
			searches.Add(1)
			var body struct {
				Query          string   `json:"query"`
				IncludeDomains []string `json:"includeDomains"`
				Start          string   `json:"startPublishedDate"`
				Contents       struct {
					Livecrawl string `json:"livecrawl"`
				} `json:"contents"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.IncludeDomains == nil {
				// A website search, as in exaServer.
				fmt.Fprint(w, `{"results":[{"title":"Probe Kampfsport Bochum","url":"https://probe-kampfsport.example/"}]}`)
				return
			}
			if fmt.Sprint(body.IncludeDomains) != "[linkedin.com/posts]" || body.Start != "2026-07-02T12:00:00Z" || body.Contents.Livecrawl != "never" {
				t.Errorf("exa post search %+v", body)
			}
			fmt.Fprint(w, `{"results":[
				{"title":"Ein Jahr selbstständig, und vieles kam anders. | Erika Beispiel","url":"https://www.linkedin.com/posts/erika-beispiel_ein-jahr-activity-1","publishedDate":"2026-08-13T00:00:00.000Z","text":"Ein Jahr selbstständig"},
				{"title":"","url":"https://de.linkedin.com/posts/max-probe_neues-studio-activity-2","publishedDate":null,"text":"# Unser Studio ist eröffnet! · 2026-09-14T00:00:00+00:00\n\nVor 1,5 Jahren"}],
				"costDollars":{"total":0.007}}`)
		case "/contents":
			contents.Add(1)
			var body struct {
				URLs      []string `json:"urls"`
				Livecrawl string   `json:"livecrawl"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Livecrawl != "never" || len(body.URLs) == 0 {
				t.Errorf("exa contents %+v", body)
			}
			fmt.Fprint(w, `{"results":[
				{"title":"Erika Beispiel","url":"https://www.linkedin.com/in/erika-beispiel","text":"# Erika Beispiel\n\nYogalehrerin und Inhaberin | Beispiel Yoga\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n120 connections • 130 followers\n\n## About",
				 "entities":[{"type":"person","properties":{"name":"Erika Beispiel","location":"Cologne, North Rhine-Westphalia, Germany"}}]},
				{"title":"Max Probe","url":"https://www.linkedin.com/in/max-probe","text":"# Max Probe\n\nRuhr Region (DE)\n\n58 connections • 61 followers"},
				{"title":"Lena Muster","url":"https://www.linkedin.com/in/lena-muster","text":"# Lena Muster\n\nCoach für Neuanfänge\n\n12 connections • 12 followers"}],
				"statuses":[{"id":"https://www.linkedin.com/in/unbekannt","status":"error","error":{"httpStatusCode":404,"tag":"ENTITY_NOT_FOUND"}}],
				"costDollars":{"total":0.003}}`)
		default:
			t.Errorf("exa path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExaPosts(t *testing.T) {
	var s, c atomic.Int32
	srv := exaPostServer(t, &s, &c)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	posts, err := exa.Posts(t.Context(), "Ein Jahr selbstständig #köln", now.AddDate(0, 0, -90))
	if err != nil {
		t.Fatal(err)
	}
	want := []search.Post{
		{URL: "https://www.linkedin.com/posts/erika-beispiel_ein-jahr-activity-1", Title: "Ein Jahr selbstständig, und vieles kam anders.",
			Published: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), Author: "https://www.linkedin.com/in/erika-beispiel"},
		{URL: "https://de.linkedin.com/posts/max-probe_neues-studio-activity-2", Title: "Unser Studio ist eröffnet!",
			Author: "https://www.linkedin.com/in/max-probe"},
	}
	if !slices.EqualFunc(posts, want, func(a, b search.Post) bool { return a == b }) {
		t.Errorf("posts\n%+v\nwant\n%+v", posts, want)
	}

	profiles, err := exa.Profiles(t.Context(), []string{"https://www.linkedin.com/in/erika-beispiel", "https://www.linkedin.com/in/max-probe"})
	if err != nil {
		t.Fatal(err)
	}
	wantP := []search.Profile{
		{URL: "https://www.linkedin.com/in/erika-beispiel", Name: "Erika Beispiel", Headline: "Yogalehrerin und Inhaberin | Beispiel Yoga", Location: "Cologne, North Rhine-Westphalia, Germany"},
		{URL: "https://www.linkedin.com/in/max-probe", Name: "Max Probe", Location: "Ruhr Region"},
		{URL: "https://www.linkedin.com/in/lena-muster", Name: "Lena Muster", Headline: "Coach für Neuanfänge"},
	}
	if !slices.Equal(profiles, wantP) {
		t.Errorf("profiles\n%+v\nwant\n%+v", profiles, wantP)
	}
	if s.Load() != 1 || c.Load() != 1 {
		t.Errorf("%d searches, %d contents calls", s.Load(), c.Load())
	}
}

func postPool(t *testing.T, exaBudget int) (*search.Pool, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	db := dbtest.New(t)
	if _, err := db.Exec(t.Context(), `UPDATE settings SET value = to_jsonb($1::int) WHERE key = 'exa_monthly_searches'`, exaBudget); err != nil {
		t.Fatal(err)
	}
	var s, c, tc atomic.Int32
	es, ts := exaPostServer(t, &s, &c), tavilyServer(t, &tc, http.StatusOK)
	return &search.Pool{
		DB: db,
		Providers: []search.Provider{
			&search.Tavily{Key: "tvly-test", URL: ts.URL + "/search"},
			&search.Exa{Key: "exa-test", URL: es.URL + "/search"},
		},
		Now: func() time.Time { return now },
	}, &s, &c, &tc
}

// Only Exa finds posts. Tavily, though it has the larger share of its
// budget left, is never asked. A lookup of several profiles is one call.
func TestPoolPostsGoOnlyToExa(t *testing.T) {
	p, s, c, tc := postPool(t, 3)
	if !p.FindsPosts() {
		t.Fatal("Exa has a key, posts can be found")
	}
	posts, from, err := p.Posts(t.Context(), "Ein Jahr selbstständig #köln", now.AddDate(0, 0, -90))
	if err != nil || from != "exa" || len(posts) != 2 {
		t.Fatalf("%v from %s: %v", posts, from, err)
	}
	if _, _, err := p.Profiles(t.Context(), []string{posts[0].Author, posts[1].Author}); err != nil {
		t.Fatal(err)
	}
	if s.Load() != 1 || c.Load() != 1 || tc.Load() != 0 {
		t.Errorf("exa %d searches and %d lookups, tavily %d", s.Load(), c.Load(), tc.Load())
	}
	// The third call spends Exa's budget. Then posts wait.
	if _, _, err := p.Posts(t.Context(), "Mein Weg zum Coach #düsseldorf", now.AddDate(0, 0, -90)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Posts(t.Context(), "Neues Studio #bonn", now.AddDate(0, 0, -90)); !errors.Is(err, search.ErrNoBudget) {
		t.Errorf("after the budget: %v", err)
	}
	usage, _ := p.Usage(t.Context())
	if usage[0].Provider != "exa" || usage[0].Used != 3 {
		t.Errorf("usage %v", usage)
	}

	// Without Exa nothing can find posts, and nothing is called.
	p.Providers = p.Providers[:1]
	if p.FindsPosts() {
		t.Error("Tavily cannot find posts")
	}
	if _, _, err := p.Posts(t.Context(), "Neues Studio #bonn", now.AddDate(0, 0, -90)); !errors.Is(err, search.ErrNoBudget) {
		t.Errorf("without Exa: %v", err)
	}
}

// Website searches, post searches and profile lookups share Exa's budget.
// Many workers at once never go past it together.
func TestPoolPostsAndSearchesShareTheBudget(t *testing.T) {
	p, s, c, _ := postPool(t, 9)
	p.Providers = p.Providers[1:]
	var wg sync.WaitGroup
	var ok, refused atomic.Int32
	for i := range 30 {
		wg.Go(func() {
			var err error
			switch i % 3 {
			case 0:
				_, _, err = p.Search(t.Context(), fmt.Sprintf("Yoga Köln %d", i))
			case 1:
				_, _, err = p.Posts(t.Context(), fmt.Sprintf("Ein Jahr selbstständig %d", i), now.AddDate(0, 0, -90))
			default:
				_, _, err = p.Profiles(t.Context(), []string{"https://www.linkedin.com/in/erika-beispiel"})
			}
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, search.ErrNoBudget):
				refused.Add(1)
			default:
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 9 || refused.Load() != 21 || s.Load()+c.Load() != 9 {
		t.Errorf("%d answered, %d refused, %d calls made", ok.Load(), refused.Load(), s.Load()+c.Load())
	}
}
