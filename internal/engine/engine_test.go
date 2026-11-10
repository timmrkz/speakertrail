package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
)

var t0 = time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)

// finder answers every query with the same invented profiles, or holds
// each call until released when hold is set.
type finder struct {
	mu       sync.Mutex
	queries  []string
	profiles []search.Profile
	err      error
	hold     chan struct{}
	started  chan string
}

func (f *finder) People(ctx context.Context, query string, results int) ([]search.Profile, error) {
	f.mu.Lock()
	f.queries = append(f.queries, query)
	f.mu.Unlock()
	if f.started != nil {
		f.started <- query
	}
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.profiles, f.err
}

func steps(title, org string, from int, school string, schoolFrom int) []search.Step {
	return []search.Step{{Title: title, Org: org, From: from}, {Education: true, Title: "Bachelor", Org: school, From: schoolFrom, To: schoolFrom + 3}}
}

// Invented people. One runs her own studio in Köln, one is a consultant,
// one lives in Hamburg, one was last active two years ago.
var invented = []search.Profile{
	{URL: "https://de.linkedin.com/in/lea-beispiel/", Name: "Lea Beispiel", Headline: "Gründerin, Studio Beispiel", Location: "Köln, Nordrhein-Westfalen, Deutschland",
		Career: steps("Gründerin", "Studio Beispiel", 2023, "Universität zu Köln", 2019), LastActive: t0.AddDate(0, -1, 0)},
	{URL: "https://www.linkedin.com/in/udo-berater", Name: "Udo Berater", Headline: "Unternehmensberater", Location: "Düsseldorf, Germany",
		Career: steps("Unternehmensberater", "Beratung GmbH", 2010, "RWTH Aachen", 2000), LastActive: t0.AddDate(0, -1, 0)},
	{URL: "https://www.linkedin.com/in/hanna-nord", Name: "Hanna Nord", Headline: "Inhaberin, Yoga Nord", Location: "Hamburg, Germany",
		Career: steps("Inhaberin", "Yoga Nord", 2021, "Universität Hamburg", 2015), LastActive: t0.AddDate(0, -1, 0)},
	{URL: "https://www.linkedin.com/in/stille-sam", Name: "Sam Stille", Headline: "Gründer, Stille GmbH", Location: "Bonn, Germany",
		Career: steps("Gründer", "Stille GmbH", 2020, "Universität Bonn", 2014), LastActive: t0.AddDate(-2, 0, 0)},
}

func newEngine(t *testing.T, f *finder) (*Engine, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir(), func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(d *store.Data) error {
		_, err := d.AddSearch("Gründerin Köln", 0, t0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e := &Engine{Store: st, Finder: func() Finder {
		if f == nil {
			return nil
		}
		return f
	}}
	return e, st
}

func TestARunKeepsOnlyGuests(t *testing.T) {
	e, st := newEngine(t, &finder{profiles: invented})
	id, started, err := e.Start("manual", 0)
	if err != nil || !started {
		t.Fatalf("start: %v %v", started, err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		if len(d.People) != 1 || d.People[0].Name != "Lea Beispiel" {
			t.Fatalf("kept %+v, want only Lea", d.People)
		}
		p := d.People[0]
		if p.City != "Köln" || p.LinkedIn.URL != "https://www.linkedin.com/in/lea-beispiel" || p.LinkedIn.Roles != "Gründerin, Studio Beispiel" {
			t.Errorf("person = %+v", p)
		}
		if p.LinkedIn.BornAbout == 0 || p.Score <= 0 || len(p.Signals) == 0 {
			t.Errorf("not scored: born %d, score %d, signals %v", p.LinkedIn.BornAbout, p.Score, p.Signals)
		}
		r := d.Run(id)
		if r.FinishedAt.IsZero() || r.Ended != "" || len(r.Checks) != 1 || r.Checks[0].PeopleNew != 1 {
			t.Errorf("run = %+v", r)
		}
		s := d.Searches[0]
		if s.Status != "active" || s.Runs != 1 {
			t.Errorf("search = %+v", s)
		}
		if d.Used(Provider, t0) != 1 {
			t.Errorf("used %d Exa calls, want 1", d.Used(Provider, t0))
		}
	})
	// The search ran, so the next run has nothing due.
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrNothing) {
		t.Errorf("second start: %v, want ErrNothing", err)
	}
}

func TestAFailedSearchIsDueAgain(t *testing.T) {
	e, st := newEngine(t, &finder{err: errors.New("exa answered 500")})
	id, _, err := e.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		if c := d.Run(id).Checks; len(c) != 1 || c[0].Error == "" {
			t.Errorf("checks = %+v", c)
		}
		if len(d.Due(t0, 10)) != 1 {
			t.Error("a search that failed is not due again")
		}
	})
}

func TestNoRunWithoutAKey(t *testing.T) {
	e, _ := newEngine(t, nil)
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrNoKey) {
		t.Errorf("err = %v, want ErrNoKey", err)
	}
}

func TestNoRunWithoutBudget(t *testing.T) {
	e, st := newEngine(t, &finder{})
	_ = st.Update(func(d *store.Data) error { return d.SetSetting("exa_monthly_searches", []byte("0"), t0) })
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrNoBudget) {
		t.Errorf("err = %v, want ErrNoBudget", err)
	}
}

func TestStopEndsTheRunAndLeavesTheRestDue(t *testing.T) {
	f := &finder{hold: make(chan struct{}), started: make(chan string, 16)}
	e, st := newEngine(t, f)
	_ = st.Update(func(d *store.Data) error {
		for _, q := range []string{"Yogalehrerin Bonn", "Personal Trainer Essen", "Coach Münster", "Tattoo Studio Dortmund", "Bäckerei Aachen"} {
			if _, err := d.AddSearch(q, 0, t0); err != nil {
				return err
			}
		}
		return nil
	})
	id, _, err := e.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	for range SideBySide {
		<-f.started
	}
	if going, now := e.Going(); going != id || len(now) != SideBySide {
		t.Errorf("going %d with %d searches, want %d with %d", going, len(now), id, SideBySide)
	}
	if !e.Stop(id) {
		t.Fatal("Stop found no run")
	}
	if going, _ := e.Going(); going != 0 {
		t.Error("a run still goes after Stop")
	}
	st.Read(func(d *store.Data) {
		r := d.Run(id)
		if r.Ended != "stopped" || r.FinishedAt.IsZero() {
			t.Errorf("run = %+v", r)
		}
		if len(d.Due(t0, 10)) != 6 {
			t.Errorf("%d searches due after Stop, want all 6", len(d.Due(t0, 10)))
		}
	})
}

// The window starts, watches and stops runs from several goroutines.
func TestStartStopAndGoingFromSeveralGoroutines(t *testing.T) {
	f := &finder{profiles: invented}
	e, st := newEngine(t, f)
	_ = st.Update(func(d *store.Data) error {
		for _, q := range []string{"Yogalehrerin Bonn", "Personal Trainer Essen", "Coach Münster"} {
			if _, err := d.AddSearch(q, 0, t0); err != nil {
				return err
			}
		}
		return nil
	})
	var wg sync.WaitGroup
	ids := make(chan int64, 32)
	for range 8 {
		wg.Go(func() {
			for range 4 {
				id, _, err := e.Start("manual", 0)
				if err == nil {
					ids <- id
				}
				going, _ := e.Going()
				e.Stop(going)
			}
		})
	}
	wg.Wait()
	e.Wait()
	close(ids)
	st.Read(func(d *store.Data) {
		for id := range ids {
			if r := d.Run(id); r == nil || r.FinishedAt.IsZero() {
				t.Errorf("run %d did not end: %+v", id, r)
			}
		}
	})
}

func TestRecoverEndsRunsTheAppClosedUnder(t *testing.T) {
	e, st := newEngine(t, &finder{})
	_ = st.Update(func(d *store.Data) error {
		d.Runs = append(d.Runs, &store.Run{ID: d.NewID(), Kind: "manual", StartedAt: t0,
			Checks: []store.Check{{At: t0.Add(time.Minute)}}})
		return nil
	})
	if err := e.Recover(); err != nil {
		t.Fatal(err)
	}
	st.Read(func(d *store.Data) {
		r := d.Runs[0]
		if r.Ended != "interrupted" || !r.FinishedAt.Equal(t0.Add(time.Minute)) {
			t.Errorf("run = %+v", r)
		}
	})
}

func TestLinkedIn(t *testing.T) {
	for in, want := range map[string]string{
		"https://de.linkedin.com/in/lea-beispiel/":     "https://www.linkedin.com/in/lea-beispiel",
		"https://www.linkedin.com/in/lea?trk=x":        "https://www.linkedin.com/in/lea",
		"https://www.linkedin.com/company/beispiel":    "",
		"https://www.xing.com/profile/Lea_Beispiel":    "",
		"javascript:alert(1)//www.linkedin.com/in/lea": "",
	} {
		if got := linkedIn(in); got != want {
			t.Errorf("linkedIn(%q) = %q, want %q", in, got, want)
		}
	}
}
