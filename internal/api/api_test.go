package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/keys"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
)

var t0 = time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)

type memKeys struct {
	mu  sync.Mutex
	key string
}

func (k *memKeys) Has() bool { k.mu.Lock(); defer k.mu.Unlock(); return k.key != "" }
func (k *memKeys) Set(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = key
	return nil
}

type finder struct{}

func (finder) People(ctx context.Context, query string, results int) ([]search.Profile, error) {
	return []search.Profile{{
		URL: "https://www.linkedin.com/in/lea-beispiel", Name: "Lea Beispiel", Headline: "Gründerin, Studio Beispiel",
		Location: "Köln, Germany", LastActive: t0.AddDate(0, -1, 0),
		Career: []search.Step{{Title: "Gründerin", Org: "Studio Beispiel", From: 2023}, {Education: true, Title: "Bachelor", Org: "Universität zu Köln", From: 2019, To: 2022}},
	}}, nil
}

// ProfileByName finds nobody by name.
func (finder) ProfileByName(context.Context, string) (search.Profile, error) {
	return search.Profile{}, search.ErrNotIndexed
}

// Profile reads one invented profile, and no other link is in the index.
func (finder) Profile(ctx context.Context, link string) (search.Profile, error) {
	if link != "https://www.linkedin.com/in/ida-probe" {
		return search.Profile{}, search.ErrNotIndexed
	}
	return search.Profile{
		URL: link, Name: "Ida Probe", Headline: "Inhaberin, Probe Studio", Location: "Essen, Germany", LastActive: t0.AddDate(0, -1, 0),
		Text: "Ida Probe\nInhaberin, Probe Studio",
	}, nil
}

func newAPI(t *testing.T) (*API, *store.Store, *engine.Engine) {
	t.Helper()
	st, err := store.Open(t.TempDir(), func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	keys := &memKeys{key: "invented"}
	e := &engine.Engine{Store: st, Finder: func() (engine.Finder, error) {
		if !keys.Has() {
			return nil, engine.ErrNoKey
		}
		return finder{}, nil
	}}
	return New(st, e, keys, nil), st, e
}

func call(t *testing.T, h http.Handler, method, path, body string, out any) int {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v in %s", method, path, err, rec.Body)
		}
	}
	return rec.Code
}

func TestARunFromStartToPeople(t *testing.T) {
	a, _, e := newAPI(t)
	var added searchJSON
	if code := call(t, a, "POST", "/api/sources", `{"query":"Gründerin Köln","kind":"profile_search"}`, &added); code != http.StatusCreated {
		t.Fatalf("add: %d", code)
	}
	if added.Query != "Gründerin Köln" || added.City != "Köln" || added.Health != "never" {
		t.Errorf("added = %+v", added)
	}
	if code := call(t, a, "POST", "/api/sources", `{"query":"gründerin köln"}`, nil); code != http.StatusConflict {
		t.Errorf("the same search again: %d", code)
	}

	var started struct {
		RunID   int64 `json:"run_id"`
		Started bool  `json:"started"`
	}
	if code := call(t, a, "POST", "/api/runs", `{}`, &started); code != http.StatusOK || !started.Started {
		t.Fatalf("start: %d %+v", code, started)
	}
	e.Wait()
	var current struct{ Run *runJSON }
	call(t, a, "GET", "/api/runs/current", "", &current)
	if current.Run != nil {
		t.Errorf("a run still goes: %+v", current.Run)
	}
	var run struct {
		Run    runJSON
		Checks []checkJSON
	}
	if code := call(t, a, "GET", "/api/runs/"+itoa(started.RunID), "", &run); code != http.StatusOK {
		t.Fatalf("run: %d", code)
	}
	if run.Run.State != "finished" || run.Run.PeopleNew != 1 || run.Run.SourcesChecked != 1 || len(run.Checks) != 1 {
		t.Errorf("run = %+v", run)
	}

	var people struct {
		People  []personJSON
		Counts  map[string]int
		NewDays int `json:"new_days"`
	}
	call(t, a, "GET", "/api/people?sort=young&filter=all", "", &people)
	if len(people.People) != 1 || people.Counts["connected"] != 0 || people.Counts["all"] != 1 || people.NewDays != 7 {
		t.Fatalf("people = %+v", people)
	}
	p := people.People[0]
	if p.Name != "Lea Beispiel" || p.LinkedIn == nil || p.LinkedIn.BornAbout == nil || len(p.Profiles) != 1 {
		t.Errorf("person = %+v", p)
	}

	var detail personDetail
	if code := call(t, a, "PATCH", "/api/people/"+itoa(p.ID), `{"connected":true,"notes":"Ask about the studio"}`, &detail); code != http.StatusOK {
		t.Fatalf("connect: %d", code)
	}
	if !detail.Connected || detail.Notes != "Ask about the studio" || len(detail.Sightings) != 1 || detail.Sightings[0].Source != "Gründerin Köln" {
		t.Errorf("detail = %+v", detail)
	}
	// The fit is its parts, the same in the list and on the person.
	sum := 0
	for _, part := range detail.FitParts {
		sum += part.Points
	}
	if len(detail.FitParts) != 4 || detail.FitScore != sum || p.FitScore != sum || sum <= 0 {
		t.Errorf("fit %d in the list, %d on the person, parts %+v", p.FitScore, detail.FitScore, detail.FitParts)
	}
	// New holds her for as many days as the setting new_days says, at
	// least one.
	if code := call(t, a, "PATCH", "/api/settings/new_days", `{"value":0}`, nil); code != http.StatusBadRequest {
		t.Errorf("new for no day: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/settings/new_days", `{"value":3}`, nil); code != http.StatusOK {
		t.Fatalf("new for 3 days: %d", code)
	}
	call(t, a, "GET", "/api/people?filter=new", "", &people)
	if len(people.People) != 1 || people.Counts["new"] != 1 || people.NewDays != 3 {
		t.Errorf("new for 3 days = %+v", people)
	}
	if code := call(t, a, "PATCH", "/api/settings/new_days", `{"value":2.5}`, nil); code != http.StatusBadRequest {
		t.Errorf("a part of a day: %d", code)
	}
	call(t, a, "PATCH", "/api/settings/new_days", `{"value":7}`, nil)
	// Connected, she leaves All for Connected.
	call(t, a, "GET", "/api/people?filter=all", "", &people)
	if len(people.People) != 0 || people.Counts["all"] != 0 || people.Counts["new"] != 1 || people.Counts["connected"] != 1 {
		t.Errorf("people once connected = %+v", people)
	}
	call(t, a, "GET", "/api/people?filter=connected", "", &people)
	if len(people.People) != 1 || people.People[0].ID != p.ID {
		t.Errorf("connected = %+v", people)
	}
	if code := call(t, a, "PATCH", "/api/people/"+itoa(p.ID), `{"connected":"maybe"}`, nil); code != http.StatusBadRequest {
		t.Errorf("connected that is no yes or no: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/people/999", `{"connected":true}`, nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
	}
	// Not connected and her profile rejected, she is only under Rejected.
	call(t, a, "PATCH", "/api/people/"+itoa(p.ID), `{"connected":false}`, nil)
	if code := call(t, a, "PATCH", "/api/profiles/"+itoa(p.ID), `{"review":"rejected"}`, nil); code != http.StatusNoContent {
		t.Fatalf("reject: %d", code)
	}
	call(t, a, "GET", "/api/people?filter=rejected", "", &people)
	if len(people.People) != 1 || people.People[0].ID != p.ID || people.Counts["all"] != 0 || people.Counts["connected"] != 0 || people.Counts["rejected"] != 1 {
		t.Errorf("people once rejected = %+v", people)
	}
	call(t, a, "PATCH", "/api/profiles/"+itoa(p.ID), `{"review":"confirmed"}`, nil)
	call(t, a, "PATCH", "/api/people/"+itoa(p.ID), `{"connected":true}`, nil)

	var searches struct{ Sources []searchJSON }
	call(t, a, "GET", "/api/sources?kind=profile_search", "", &searches)
	if len(searches.Sources) != 1 || searches.Sources[0].People != 1 || searches.Sources[0].PeopleConnected != 1 || searches.Sources[0].Status != "active" {
		t.Errorf("searches = %+v", searches)
	}
}

func TestPauseAndResume(t *testing.T) {
	a, _, _ := newAPI(t)
	var s searchJSON
	call(t, a, "POST", "/api/sources", `{"query":"Yogalehrerin Bonn","kind":"profile_search"}`, &s)
	call(t, a, "PATCH", "/api/sources/"+itoa(s.ID), `{"status":"retired"}`, &s)
	if s.Status != "retired" {
		t.Errorf("paused: %s", s.Status)
	}
	if code := call(t, a, "POST", "/api/runs", `{}`, nil); code != http.StatusConflict {
		t.Errorf("a run with every search paused: %d", code)
	}
	call(t, a, "PATCH", "/api/sources/"+itoa(s.ID), `{"status":"candidate"}`, &s)
	if s.Status != "candidate" {
		t.Errorf("resumed: %s", s.Status)
	}
}

func TestSettingsAndKeys(t *testing.T) {
	a, _, _ := newAPI(t)
	var set store.Setting
	if code := call(t, a, "PATCH", "/api/settings/profile_searches_per_run", `{"value":5}`, &set); code != http.StatusOK || string(set.Value) != "5" {
		t.Errorf("set: %d %s", code, set.Value)
	}
	if code := call(t, a, "PATCH", "/api/settings/profile_searches_per_run", `{"value":"many"}`, nil); code != http.StatusBadRequest {
		t.Errorf("text for a number: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/settings/nothing_like_it", `{"value":1}`, nil); code != http.StatusNotFound {
		t.Errorf("unknown setting: %d", code)
	}
	var keys struct{ Keys []store.Use }
	call(t, a, "GET", "/api/keys", "", &keys)
	if len(keys.Keys) != 1 || !keys.Keys[0].Set || keys.Keys[0].Budget != 1000 {
		t.Errorf("keys = %+v", keys)
	}
	if code := call(t, a, "PUT", "/api/keys/exa", `{"key":""}`, nil); code != http.StatusNoContent {
		t.Errorf("remove the key: %d", code)
	}
	if code := call(t, a, "POST", "/api/runs", `{}`, nil); code != http.StatusConflict {
		t.Errorf("a run without a key: %d", code)
	}
}

// A run whose key could not be read is refused with why, the way a run
// without a key is, never as something gone wrong inside.
func TestARunWhoseKeyCouldNotBeReadSaysWhy(t *testing.T) {
	a, _, e := newAPI(t)
	call(t, a, "POST", "/api/sources", `{"query":"Yogalehrerin Bonn","kind":"profile_search"}`, nil)
	e.Finder = func() (engine.Finder, error) {
		return nil, keys.ErrUnreadable
	}
	var refused struct{ Error string }
	want := "The Exa key's file could not be read. Save the key again in Settings"
	if code := call(t, a, "POST", "/api/runs", `{}`, &refused); code != http.StatusConflict || refused.Error != want {
		t.Errorf("%d %q, want %q", code, refused.Error, want)
	}
}

func TestOnlyJSONChangesAnything(t *testing.T) {
	a, _, _ := newAPI(t)
	req := httptest.NewRequest("POST", "/api/runs", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("code %d", rec.Code)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestExclusions(t *testing.T) {
	a, _, _ := newAPI(t)
	var list exclusionsJSON
	if code := call(t, a, "GET", "/api/exclusions", "", &list); code != http.StatusOK || len(list.Exclusions) != 6 || list.Exclusions[0].Phrase != "LinkedIn" {
		t.Fatalf("the starting exclusions: %d %+v", code, list)
	}
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"  Business   Coach "}`, &list); code != http.StatusCreated || len(list.Exclusions) != 7 || list.Exclusions[0].Phrase != "Business Coach" || list.Exclusions[0].People != 0 {
		t.Fatalf("added: %d %+v", code, list)
	}
	x := list.Exclusions[0]
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"business coach"}`, nil); code != http.StatusConflict {
		t.Errorf("the same phrase again: %d", code)
	}
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"--"}`, nil); code != http.StatusBadRequest {
		t.Errorf("a phrase without a word: %d", code)
	}
	if code := call(t, a, "DELETE", "/api/exclusions/"+itoa(x.ID), "", &list); code != http.StatusOK || len(list.Exclusions) != 6 || list.Exclusions[0].Phrase != "LinkedIn" {
		t.Errorf("delete: %d %+v", code, list)
	}
	if code := call(t, a, "DELETE", "/api/exclusions/"+itoa(x.ID), "", nil); code != http.StatusNotFound {
		t.Errorf("delete again: %d", code)
	}
}

// The window showed exclusions in its own order after adding one, so one
// that starts with an umlaut jumped on the next load, and kept the old
// counts after adding or removing one, though a person counts for the
// first exclusion that matches, issue #105. Adding and removing now
// answer the list in German order, Ä with the A, with the counts and how
// many people all of them hide.
func TestExclusionsAnswerTheirOrderAndCounts(t *testing.T) {
	a, st, _ := newAPI(t)
	st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Coach Köln", 0, t0)
		d.AddFound(store.Found{URL: "https://www.linkedin.com/in/ida-beispiel", Name: "Ida Beispiel", Headline: "Online Marketing Coach"}, s, 1, t0)
		d.AddFound(store.Found{URL: "https://www.linkedin.com/in/ole-beispiel", Name: "Ole Beispiel", Headline: "Ärztin und Coach"}, s, 1, t0)
		return nil
	})
	var list exclusionsJSON
	phrases := func() string {
		var out []string
		for _, x := range list.Exclusions {
			out = append(out, fmt.Sprintf("%s %d", x.Phrase, x.People))
		}
		return fmt.Sprintf("%s, hiding %d", strings.Join(out, ", "), list.Hiding)
	}
	want := func(step, says string) {
		t.Helper()
		if got := phrases(); got != says {
			t.Errorf("%s: %s, want %s", step, got, says)
		}
	}
	call(t, a, "GET", "/api/exclusions", "", &list)
	want("at the start", "LinkedIn 0, Online Marketing 1, Performance Marketing 0, Personal Branding 0, Sichtbarkeit 0, Social Media Marketing 0, hiding 1")
	call(t, a, "POST", "/api/exclusions", `{"phrase":"Coach"}`, &list)
	want("Coach added", "Coach 2, LinkedIn 0, Online Marketing 0, Performance Marketing 0, Personal Branding 0, Sichtbarkeit 0, Social Media Marketing 0, hiding 2")
	coach := list.Exclusions[0].ID
	call(t, a, "POST", "/api/exclusions", `{"phrase":"Ärztin"}`, &list)
	want("Ärztin added", "Ärztin 1, Coach 1, LinkedIn 0, Online Marketing 0, Performance Marketing 0, Personal Branding 0, Sichtbarkeit 0, Social Media Marketing 0, hiding 2")
	var again exclusionsJSON
	call(t, a, "GET", "/api/exclusions", "", &again)
	if fmt.Sprint(again) != fmt.Sprint(list) {
		t.Errorf("a reload gave %+v, want what adding answered, %+v", again, list)
	}
	call(t, a, "DELETE", "/api/exclusions/"+itoa(coach), "", &list)
	want("Coach removed", "Ärztin 1, LinkedIn 0, Online Marketing 1, Performance Marketing 0, Personal Branding 0, Sichtbarkeit 0, Social Media Marketing 0, hiding 2")

	// Several at once, as Tim's clicks and a run may come together.
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			var got exclusionsJSON
			if code := call(t, a, "POST", "/api/exclusions", fmt.Sprintf(`{"phrase":"Ölmalerei %d"}`, i), &got); code != http.StatusCreated {
				t.Errorf("adding %d: %d", i, code)
			}
			call(t, a, "GET", "/api/exclusions", "", &got)
		})
	}
	wg.Wait()
	call(t, a, "GET", "/api/exclusions", "", &list)
	if len(list.Exclusions) != 13 || list.Exclusions[2].Phrase != "Ölmalerei 0" || list.Exclusions[7].Phrase != "Ölmalerei 5" {
		t.Errorf("after six at once: %s", phrases())
	}
}
