package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
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

func newAPI(t *testing.T) (*API, *store.Store, *engine.Engine) {
	t.Helper()
	st, err := store.Open(t.TempDir(), func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	keys := &memKeys{key: "invented"}
	e := &engine.Engine{Store: st, Finder: func() engine.Finder {
		if !keys.Has() {
			return nil
		}
		return finder{}
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
		People []personJSON
		Counts map[string]int
	}
	call(t, a, "GET", "/api/people?sort=young&filter=all", "", &people)
	if len(people.People) != 1 || people.Counts["connected"] != 0 || people.Counts["all"] != 1 {
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
	if code := call(t, a, "PATCH", "/api/people/"+itoa(p.ID), `{"connected":"maybe"}`, nil); code != http.StatusBadRequest {
		t.Errorf("connected that is no yes or no: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/people/999", `{"connected":true}`, nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
	}

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
	var list []exclusionJSON
	if code := call(t, a, "GET", "/api/exclusions", "", &list); code != http.StatusOK || len(list) != 6 || list[0].Phrase != "LinkedIn" {
		t.Fatalf("the starting exclusions: %d %+v", code, list)
	}
	var x exclusionJSON
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"  Business   Coach "}`, &x); code != http.StatusCreated || x.Phrase != "Business Coach" || x.People != 0 {
		t.Fatalf("added: %d %+v", code, x)
	}
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"business coach"}`, nil); code != http.StatusConflict {
		t.Errorf("the same phrase again: %d", code)
	}
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase":"--"}`, nil); code != http.StatusBadRequest {
		t.Errorf("a phrase without a word: %d", code)
	}
	if code := call(t, a, "DELETE", "/api/exclusions/"+itoa(x.ID), "", nil); code != http.StatusNoContent {
		t.Errorf("delete: %d", code)
	}
	if code := call(t, a, "DELETE", "/api/exclusions/"+itoa(x.ID), "", nil); code != http.StatusNotFound {
		t.Errorf("delete again: %d", code)
	}
}
