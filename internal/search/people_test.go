package search_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
				{"title":"Erika Beispiel","url":"https://www.linkedin.com/in/erika-beispiel","text":"# Erika Beispiel\n\nYogalehrerin und Inhaberin | Beispiel Yoga\n\nCologne, North Rhine-Westphalia, Germany (DE)\n\n120 connections • 130 followers\n\n## About",
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
	}}
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
