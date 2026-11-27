package search_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/search"
)

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
				Category       string   `json:"category"`
				NumResults     int      `json:"numResults"`
				IncludeDomains []string `json:"includeDomains"`
				Start          string   `json:"startPublishedDate"`
				Contents       struct {
					Livecrawl string `json:"livecrawl"`
				} `json:"contents"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Category == "people" {
				if body.Contents.Livecrawl != "never" || body.NumResults != 25 {
					t.Errorf("exa people search %+v", body)
				}
				// A profile with its activity last, and a page that is no
				// LinkedIn profile.
				fmt.Fprint(w, `{"results":[
					{"title":"Lea Probe","url":"https://de.linkedin.com/in/lea-probe","text":"# Lea Probe\n\nPersonal Trainerin | Selbstständig\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n300 connections\n\n## Activity\n\n- [2026-06-02 · Gefällt mir: Ein Beitrag](https://www.linkedin.com/posts/x)\n- [2026-10-30 · Kommentar: Starker Kurs!](https://www.linkedin.com/posts/y)",
					 "entities":[{"type":"person","properties":{"name":"Lea Probe","location":"Cologne, North Rhine-Westphalia, Germany",
					   "workHistory":[{"title":"Personal Trainerin","dates":{"from":"2023-01-01","to":null},"company":{"name":"Selbstständig"}}],"educationHistory":[]}}]},
					{"title":"Probe Fitness Team","url":"https://probe-fitness.example/team","text":"Unser Team"}],
					"costDollars":{"total":0.007}}`)
				return
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
				{"title":"Erika Beispiel","url":"https://www.linkedin.com/in/erika-beispiel","text":"# Erika Beispiel\n\nYogalehrerin und Inhaberin | Beispiel Yoga\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n120 connections • 130 followers\n\n## About\n\nKurse in Köln. Anmeldung: erika@beispiel-yoga.de, Tel. 0221 1234567",
				 "entities":[{"type":"person","properties":{"name":"Erika Beispiel","location":"Cologne, North Rhine-Westphalia, Germany",
				  "workHistory":[{"title":"Inhaberin","location":null,"dates":{"from":"2023-04-01","to":null},"company":{"id":null,"name":"Beispiel Yoga"}},
				    {"title":"Werkstudentin","dates":{"from":"2019-10-01","to":"2022-03-01"},"company":{"name":"Muster GmbH"}},
				    {"title":"Yogalehrerin","dates":null,"company":{"name":"Probe Studio"}}],
				  "educationHistory":[{"degree":"Bachelor of Arts","dates":{"from":"2018","to":"2022"},"institution":{"id":null,"name":"Beispiel Hochschule"}},
				    {"degree":"Yoga Teacher Training","dates":{"from":null,"to":null},"institution":{"name":"Probe Ashram"}}],"research":null}}]},
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

func TestExaPeople(t *testing.T) {
	var s, c atomic.Int32
	srv := exaPostServer(t, &s, &c)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	people, err := exa.People(t.Context(), "Personal Trainer Köln", 25)
	if err != nil {
		t.Fatal(err)
	}
	want := []search.Profile{{
		URL: "https://de.linkedin.com/in/lea-probe", Name: "Lea Probe", Headline: "Personal Trainerin | Selbstständig",
		Location: "Cologne, North Rhine-Westphalia, Germany", Career: []search.Step{{Title: "Personal Trainerin", Org: "Selbstständig", From: 2023}},
		LastActive: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		// The whole text is kept, since a first message is written from it.
		Text: "# Lea Probe\n\nPersonal Trainerin | Selbstständig\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n300 connections\n\n## Activity\n\n- [2026-06-02 · Gefällt mir: Ein Beitrag](https://www.linkedin.com/posts/x)\n- [2026-10-30 · Kommentar: Starker Kurs!](https://www.linkedin.com/posts/y)",
	}}
	// Exa gave the text whole, so as long as what is kept.
	want[0].Length = len([]rune(want[0].Text))
	if !reflect.DeepEqual(people, want) {
		t.Errorf("people\n%+v\nwant\n%+v", people, want)
	}
	if s.Load() != 1 || c.Load() != 0 {
		t.Errorf("%d searches, %d contents calls", s.Load(), c.Load())
	}
}

// Exa sometimes fails on its side for a moment. A profile search asks once
// more before it gives up, and a second failure is an error.
func TestExaPeopleAsksAgainOnce(t *testing.T) {
	for _, fails := range []int32{1, 2} {
		var s, c, failed atomic.Int32
		inner := exaPostServer(t, &s, &c)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if failed.Add(1) <= fails {
				http.Error(w, `{"error":"INTERNAL_ERROR"}`, http.StatusInternalServerError)
				return
			}
			inner.Config.Handler.ServeHTTP(w, r)
		}))
		t.Cleanup(srv.Close)
		exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search", RetryWait: time.Millisecond}
		people, err := exa.People(t.Context(), "Personal Trainer Köln", 25)
		switch {
		case fails == 1 && (err != nil || len(people) != 1):
			t.Errorf("after one failure: %d people, %v", len(people), err)
		case fails == 2 && err == nil:
			t.Errorf("after two failures no error")
		}
		if failed.Load() != 2 {
			t.Errorf("%d failures: %d calls, want 2", fails, failed.Load())
		}
	}
}

// A person whose text the app lacks has it read from Exa's index by the
// link, never from LinkedIn live.
func TestExaProfile(t *testing.T) {
	var s, c atomic.Int32
	srv := exaPostServer(t, &s, &c)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	p, err := exa.Profile(t.Context(), "https://www.linkedin.com/in/erika-beispiel")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Erika Beispiel" || p.Headline != "Yogalehrerin und Inhaberin | Beispiel Yoga" ||
		!strings.HasPrefix(p.Text, "# Erika Beispiel") || !strings.HasSuffix(p.Text, "Kurse in Köln. Anmeldung: , Tel.") {
		t.Errorf("profile %+v", p)
	}
	if s.Load() != 0 || c.Load() != 1 {
		t.Errorf("%d searches, %d contents calls", s.Load(), c.Load())
	}
}

// A profile Exa's index does not hold is ErrNotIndexed, said without the
// link, since a link is personal data and the error reaches the log.
func TestExaProfileNotIndexedNamesNoLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	_, err := exa.Profile(t.Context(), "https://www.linkedin.com/in/erika-beispiel")
	if !errors.Is(err, search.ErrNotIndexed) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "erika-beispiel") {
		t.Errorf("the error names the link: %v", err)
	}
}

// A profile Exa holds under another link, like de.linkedin.com, is found
// by the name in its link. A namesake under another link name is never
// taken, and the people search reads nothing live.
func TestExaProfileByName(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query    string `json:"query"`
			Category string `json:"category"`
			Contents struct {
				Livecrawl string `json:"livecrawl"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/search" || body.Category != "people" || body.Contents.Livecrawl != "never" {
			t.Errorf("exa %s %+v", r.URL.Path, body)
		}
		asked = append(asked, body.Query)
		fmt.Fprint(w, `{"results":[
			{"title":"Jörg Probe","url":"https://www.linkedin.com/in/joerg-probe","text":"# Jörg Probe\n\nSchreiner"},
			{"title":"Jörg Probe","url":"https://de.linkedin.com/in/J%C3%B6rg-Probe-7a1b2c3/","text":"# Jörg Probe\n\nInhaber | Probe Möbel\n\nBielefeld, North Rhine-Westphalia, Germany (DE)"}]}`)
	}))
	t.Cleanup(srv.Close)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	p, err := exa.ProfileByName(t.Context(), "https://www.linkedin.com/in/jörg-probe-7a1b2c3")
	if err != nil {
		t.Fatal(err)
	}
	if p.Headline != "Inhaber | Probe Möbel" {
		t.Errorf("took %+v", p)
	}
	if fmt.Sprint(asked) != "[jörg probe]" {
		t.Errorf("asked %q", asked)
	}
	// Only the namesake: nobody.
	_, err = exa.ProfileByName(t.Context(), "https://www.linkedin.com/in/joerg-probe-99")
	if !errors.Is(err, search.ErrNotIndexed) || strings.Contains(err.Error(), "probe") {
		t.Errorf("a namesake: err = %v", err)
	}
	// A link with no name in it asks nothing.
	asked = nil
	if _, err := exa.ProfileByName(t.Context(), "https://www.linkedin.com/in/ACoAAB12345"); !errors.Is(err, search.ErrNotIndexed) || asked != nil {
		t.Errorf("a link with no name: err = %v, asked %q", err, asked)
	}
}

// longProfile is a profile as Exa's index writes one of someone with a
// long career: the jobs, each with Exa's words about the company, run
// past TextChars, and the activity comes last, under Social, as in Tim's
// report of 27 November 2026.
func longProfile() string {
	var b strings.Builder
	b.WriteString("# Erika Beispiel\n\nWir helfen Unternehmern zu wachsen\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n500 connections • 5,240 followers\n\n## About\n\nCoaching für Menschen mit viel Verantwortung.\n\n## Experience\n\n")
	for i := range 60 {
		fmt.Fprintf(&b, "### Geschäftsführerin - Beispiel %d GmbH\n\nJan 2021 - May 2023 in Köln\n\nBeispiel %d GmbH is a Business Consulting and Services company with 1-10 employees, founded in 2020. Headquartered in Cologne, North Rhine-Westphalia, Germany.\n\n", i, i)
	}
	b.WriteString("## Education\n\n### Bachelor at Beispiel Hochschule\n\n2016 - 2020 (4 years)\n\n")
	b.WriteString("## Social\n\n### Linkedin\n\n")
	b.WriteString("- [2026-11-25 · Erika Beispiel posted this] [Neues Programm ab Januar...](https://www.linkedin.com/posts/a)\n")
	b.WriteString("- [2026-11-23 · Erika Beispiel liked this] [Ein Beitrag...](https://www.linkedin.com/posts/b)\n")
	return b.String()
}

// A profile whose text runs past what the app keeps still shows its
// activity, which Exa writes last. Exa is asked for more than is kept,
// the date is read from all of it, and the text kept gives up the end of
// the career before the activity. Before, the text was cut at 8000
// characters by Exa itself, and someone who posted hours ago showed No
// activity on LinkedIn known, Tim's report of 27 November 2026.
func TestExaLongProfileKeepsActivity(t *testing.T) {
	text := longProfile()
	if len([]rune(text)) <= search.TextChars+2000 {
		t.Fatalf("the profile is only %d characters", len([]rune(text)))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text     struct{ MaxCharacters int } `json:"text"`
			Contents struct {
				Text struct{ MaxCharacters int } `json:"text"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		limit := max(body.Text.MaxCharacters, body.Contents.Text.MaxCharacters)
		if limit < len([]rune(text)) {
			t.Errorf("%s asks for %d characters", r.URL.Path, limit)
		}
		limit = min(limit, len([]rune(text)))
		answer, _ := json.Marshal(map[string]any{"results": []map[string]any{{
			"title": "Erika Beispiel", "url": "https://www.linkedin.com/in/erika-beispiel", "text": string([]rune(text)[:limit]),
			"entities": []map[string]any{{"type": "person", "properties": map[string]any{"name": "Erika Beispiel", "location": "Cologne, North Rhine-Westphalia, Germany"}}},
		}}})
		_, _ = w.Write(answer)
	}))
	t.Cleanup(srv.Close)
	exa := &search.Exa{Key: "exa-test", URL: srv.URL + "/search"}
	found, err := exa.People(t.Context(), "Coach Köln", 25)
	if err != nil || len(found) != 1 {
		t.Fatalf("people search: %d, %v", len(found), err)
	}
	read, err := exa.Profile(t.Context(), "https://www.linkedin.com/in/erika-beispiel")
	if err != nil {
		t.Fatal(err)
	}
	for how, p := range map[string]search.Profile{"found": found[0], "read": read} {
		if want := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC); !p.LastActive.Equal(want) {
			t.Errorf("%s: last active %v, want %v", how, p.LastActive, want)
		}
		if n := len([]rune(p.Text)); n > search.TextChars || p.Length != len([]rune(text)) {
			t.Errorf("%s: %d characters kept of %d, Exa gave %d", how, n, p.Length, len([]rune(text)))
		}
		head := strings.Index(p.Text, "## About")
		job := strings.Index(p.Text, "### Geschäftsführerin - Beispiel 0 GmbH")
		social := strings.Index(p.Text, "## Social\n\n### Linkedin\n\n- [2026-11-25 · Erika Beispiel posted this]")
		if !strings.HasPrefix(p.Text, "# Erika Beispiel\n\nWir helfen") || head < 0 || job < head || social < job ||
			!strings.HasSuffix(p.Text, "[Ein Beitrag...](https://www.linkedin.com/posts/b)") {
			t.Errorf("%s: the text kept is\n%s", how, p.Text)
		}
		if strings.Contains(p.Text, "Beispiel 59 GmbH") {
			t.Errorf("%s: the end of the career is kept, and the activity has less room", how)
		}
	}
}
