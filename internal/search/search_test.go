package search_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/search"
)

// The test world is a day in late September 2026. All sites are invented.
var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// tavilyServer answers like Tavily's search API and checks the request is
// the one its documentation describes.
func tavilyServer(t *testing.T, calls *atomic.Int32, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/search" || r.Header.Get("Authorization") != "Bearer tvly-test" {
			t.Errorf("tavily request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Query       string `json:"query"`
			MaxResults  int    `json:"max_results"`
			SearchDepth string `json:"search_depth"`
			Country     string `json:"country"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Query == "" || body.MaxResults != 20 || body.SearchDepth != "basic" || body.Country != "germany" {
			t.Errorf("tavily body %+v", body)
		}
		if status != http.StatusOK {
			http.Error(w, `{"detail":{"error":"This request exceeds your plan's set usage limit."}}`, status)
			return
		}
		fmt.Fprintf(w, `{"query":%q,"results":[
			{"title":"Beispiel BJJ Köln","url":"https://beispiel-bjj.example/","content":"Brazilian Jiu-Jitsu in Köln-Ehrenfeld"},
			{"title":"Probe Yoga","url":"https://probe-yoga.example/kurse","content":"Yoga in Köln"}]}`, body.Query)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// braveServer answers like Brave's web search API.
func braveServer(t *testing.T, calls *atomic.Int32, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/res/v1/web/search" || r.Header.Get("X-Subscription-Token") != "brave-test" {
			t.Errorf("brave request %s %s %q", r.Method, r.URL.Path, r.Header.Get("X-Subscription-Token"))
		}
		if q.Get("q") == "" || q.Get("count") != "20" || q.Get("country") != "DE" || q.Get("search_lang") != "de" {
			t.Errorf("brave query %v", q)
		}
		if status != http.StatusOK {
			http.Error(w, `{"error":{"code":"RATE_LIMITED"}}`, status)
			return
		}
		fmt.Fprint(w, `{"type":"search","web":{"results":[
			{"title":"Muster Coaching Düsseldorf","url":"https://muster-coaching.example/","description":"Life Coaching in Düsseldorf"}]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// exaServer answers like Exa's search API.
func exaServer(t *testing.T, calls *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/search" || r.Header.Get("x-api-key") != "exa-test" {
			t.Errorf("exa request %s %s %q", r.Method, r.URL.Path, r.Header.Get("x-api-key"))
		}
		var body struct {
			Query        string `json:"query"`
			Type         string `json:"type"`
			NumResults   int    `json:"numResults"`
			UserLocation string `json:"userLocation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Query == "" || body.Type != "auto" || body.NumResults != 10 || body.UserLocation != "DE" {
			t.Errorf("exa body %+v", body)
		}
		fmt.Fprint(w, `{"requestId":"b5947044c4b78efa9552a7c89b306d95","results":[
			{"id":"https://probe-kampfsport.example/","title":"Probe Kampfsport Bochum","url":"https://probe-kampfsport.example/","publishedDate":null,"author":null}],
			"costDollars":{"total":0.007}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExa(t *testing.T) {
	var calls atomic.Int32
	srv := exaServer(t, &calls)
	got, err := (&search.Exa{Key: "exa-test", URL: srv.URL + "/search"}).Search(t.Context(), "Kampfsportschule Bochum")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://probe-kampfsport.example/" || got[0].Title != "Probe Kampfsport Bochum" {
		t.Errorf("results %+v", got)
	}
}

func TestTavily(t *testing.T) {
	var calls atomic.Int32
	srv := tavilyServer(t, &calls, http.StatusOK)
	got, err := (&search.Tavily{Key: "tvly-test", URL: srv.URL + "/search"}).Search(t.Context(), "BJJ Köln")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].URL != "https://beispiel-bjj.example/" || got[0].Title != "Beispiel BJJ Köln" || got[0].Snippet != "Brazilian Jiu-Jitsu in Köln-Ehrenfeld" {
		t.Errorf("results %+v", got)
	}
}

func TestBrave(t *testing.T) {
	var calls atomic.Int32
	srv := braveServer(t, &calls, http.StatusOK)
	got, err := (&search.Brave{Key: "brave-test", URL: srv.URL + "/res/v1/web/search"}).Search(t.Context(), "Life Coach Düsseldorf")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://muster-coaching.example/" || got[0].Snippet != "Life Coaching in Düsseldorf" {
		t.Errorf("results %+v", got)
	}
}

// A provider that refuses says why, in words.
func TestProviderErrors(t *testing.T) {
	var calls atomic.Int32
	srv := tavilyServer(t, &calls, 432)
	_, err := (&search.Tavily{Key: "tvly-test", URL: srv.URL + "/search"}).Search(t.Context(), "Yoga Köln")
	if err == nil || !strings.Contains(err.Error(), "tavily answered 432") || !strings.Contains(err.Error(), "usage limit") {
		t.Errorf("error %v", err)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("BRAVE_SEARCH_API_KEY", "brave-test")
	got := search.FromEnv()
	if len(got) != 1 || got[0].Name() != "brave" {
		t.Errorf("providers %v", got)
	}
	t.Setenv("EXA_API_KEY", " exa-test ")
	got = search.FromEnv()
	if len(got) != 2 || got[0].Name() != "exa" || got[0].(*search.Exa).Key != "exa-test" {
		t.Errorf("providers %v", got)
	}
}

func pool(t *testing.T, tavilyBudget, braveBudget int, tavilyStatus, braveStatus int) (*search.Pool, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	db := dbtest.New(t)
	for key, n := range map[string]int{"tavily_monthly_searches": tavilyBudget, "brave_monthly_searches": braveBudget} {
		if _, err := db.Exec(t.Context(), `UPDATE settings SET value = to_jsonb($2::int) WHERE key = $1`, key, n); err != nil {
			t.Fatal(err)
		}
	}
	var tc, bc atomic.Int32
	ts, bs := tavilyServer(t, &tc, tavilyStatus), braveServer(t, &bc, braveStatus)
	return &search.Pool{
		DB: db,
		Providers: []search.Provider{
			&search.Tavily{Key: "tvly-test", URL: ts.URL + "/search"},
			&search.Brave{Key: "brave-test", URL: bs.URL + "/res/v1/web/search"},
		},
		Now: func() time.Time { return now },
	}, &tc, &bc
}

// Both providers are used together: each search goes to the one with the
// most of its budget left, so both spend alike.
func TestPoolSpreadsSearchesOverProviders(t *testing.T) {
	p, tc, bc := pool(t, 4, 2, http.StatusOK, http.StatusOK)
	for i := range 6 {
		if _, _, err := p.Search(t.Context(), fmt.Sprintf("Yoga Köln %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if tc.Load() != 4 || bc.Load() != 2 {
		t.Errorf("tavily %d, brave %d, want 4 and 2", tc.Load(), bc.Load())
	}
	// Both budgets are spent. The engine stops calling them.
	if _, _, err := p.Search(t.Context(), "Yoga Köln 7"); !errors.Is(err, search.ErrNoBudget) {
		t.Errorf("after the budgets: %v", err)
	}
	if tc.Load()+bc.Load() != 6 {
		t.Error("a provider was called beyond its budget")
	}
	usage, err := p.Usage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Exa has a budget but no key here, so it is never called.
	if fmt.Sprint(usage) != "[{exa 0 1000 false} {tavily 4 4 true} {brave 2 2 true}]" {
		t.Errorf("usage %v", usage)
	}
	// A new month has new budgets.
	p.Now = func() time.Time { return now.AddDate(0, 1, 1) }
	if _, _, err := p.Search(t.Context(), "Yoga Köln 8"); err != nil {
		t.Errorf("next month: %v", err)
	}
}

// When a provider fails, the next one answers. The failed call counts
// against its budget, because it may have been billed.
func TestPoolFallsBackWhenAProviderFails(t *testing.T) {
	p, tc, bc := pool(t, 5, 5, 432, http.StatusOK)
	res, from, err := p.Search(t.Context(), "Coach Düsseldorf")
	if err != nil || from != "brave" || len(res) != 1 {
		t.Fatalf("%v from %s: %v", res, from, err)
	}
	if tc.Load() != 1 || bc.Load() != 1 {
		t.Errorf("tavily %d, brave %d", tc.Load(), bc.Load())
	}
	usage, _ := p.Usage(t.Context())
	if usage[1].Provider != "tavily" || usage[1].Used != 1 {
		t.Errorf("the failed call does not count: %v", usage)
	}
}

// A provider without a budget in Settings is never called.
func TestNoBudgetNoCalls(t *testing.T) {
	p, tc, bc := pool(t, 0, 0, http.StatusOK, http.StatusOK)
	if _, _, err := p.Search(t.Context(), "BJJ Köln"); !errors.Is(err, search.ErrNoBudget) {
		t.Errorf("err %v", err)
	}
	if tc.Load()+bc.Load() != 0 {
		t.Error("a provider without budget was called")
	}
}

// Many workers search at once. Together they never go past a budget.
func TestPoolKeepsBudgetsUnderLoad(t *testing.T) {
	p, tc, bc := pool(t, 5, 5, http.StatusOK, http.StatusOK)
	var wg sync.WaitGroup
	var ok, refused atomic.Int32
	for i := range 20 {
		wg.Go(func() {
			if _, _, err := p.Search(t.Context(), fmt.Sprintf("Yoga Köln %d", i)); err == nil {
				ok.Add(1)
			} else if errors.Is(err, search.ErrNoBudget) {
				refused.Add(1)
			} else {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 10 || refused.Load() != 10 || tc.Load() != 5 || bc.Load() != 5 {
		t.Errorf("%d answered, %d refused, tavily %d, brave %d", ok.Load(), refused.Load(), tc.Load(), bc.Load())
	}
}
