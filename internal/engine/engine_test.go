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

// Relaunch asks whether a run goes, and from then on no run starts until
// it gives up, so the quit into a new build never stops one half way.
func TestNoRunStartsWhileTheAppRelaunches(t *testing.T) {
	f := &finder{hold: make(chan struct{}), started: make(chan string, 16)}
	e, st := newEngine(t, f)
	_ = st.Update(func(d *store.Data) error {
		_, err := d.AddSearch("Yogalehrerin Bonn", 0, t0)
		return err
	})
	id, _, err := e.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	<-f.started
	if _, ok := e.CloseIfIdle(); ok {
		t.Fatal("CloseIfIdle said idle while a run goes")
	}
	e.Stop(id)
	reopen, ok := e.CloseIfIdle()
	if !ok {
		t.Fatal("CloseIfIdle said busy after the run stopped")
	}
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrClosed) {
		t.Errorf("a start while closed said %v, want ErrClosed", err)
	}
	reopen()
	if _, started, err := e.Start("manual", 0); err != nil || !started {
		t.Errorf("a start after reopen: %v %v", started, err)
	}
	close(f.hold)
	e.Wait()
}

// Starts and the look Relaunch makes come from several goroutines at once.
// Whenever CloseIfIdle says idle, no run goes until reopen.
func TestCloseIfIdleAndStartFromSeveralGoroutines(t *testing.T) {
	e, st := newEngine(t, &finder{profiles: invented})
	_ = st.Update(func(d *store.Data) error {
		for _, q := range []string{"Yogalehrerin Bonn", "Personal Trainer Essen", "Coach Münster"} {
			if _, err := d.AddSearch(q, 0, t0); err != nil {
				return err
			}
		}
		return nil
	})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 8 {
				id, _, _ := e.Start("manual", 0)
				e.Stop(id)
			}
		})
		wg.Go(func() {
			for range 8 {
				reopen, ok := e.CloseIfIdle()
				if ok {
					if going, _ := e.Going(); going != 0 {
						t.Errorf("run %d goes while closed", going)
					}
				}
				reopen()
			}
		})
	}
	wg.Wait()
	e.Wait()
}

// An exclusion leaves out a person who would otherwise be a guest, in
// whatever way the phrase is written.
func TestARunLeavesOutExclusions(t *testing.T) {
	mia := search.Profile{URL: "https://www.linkedin.com/in/mia-muster", Name: "Mia Muster", Headline: "Inhaberin, Studio Muster · #personalbranding",
		Location: "Köln, Germany", Career: steps("Inhaberin", "Studio Muster", 2022, "Universität zu Köln", 2018), LastActive: t0.AddDate(0, -1, 0)}
	if !Guest(mia, t0, t0.AddDate(0, -6, 0), nil) {
		t.Fatal("Mia should be a guest without exclusions")
	}
	if Guest(mia, t0, t0.AddDate(0, -6, 0), []string{"Personal Branding"}) {
		t.Fatal("Mia should be left out by Personal Branding")
	}
	e, st := newEngine(t, &finder{profiles: append([]search.Profile{mia}, invented...)})
	if _, _, err := e.Start("manual", 0); err != nil {
		t.Fatal(err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		if len(d.People) != 1 || d.People[0].Name != "Lea Beispiel" {
			t.Errorf("kept %d people, want only Lea", len(d.People))
		}
	})
}

// An age the headline gives beats the guess from the years on the
// profile.
func TestTheAgeTheHeadlineGivesWins(t *testing.T) {
	nora := search.Profile{Headline: "19 year old founder", Career: steps("Founder", "Jung Studio", 2025, "Universität Bonn", 2010)}
	born, reason, _ := careerOf(nora, t0)
	if born != 2007 || reason != "Headline says 19 in 2026" {
		t.Errorf("born %d, %q", born, reason)
	}
	born, _, _ = careerOf(invented[0], t0)
	if born == 0 || born == 2007 {
		t.Errorf("without an age in the headline, born %d", born)
	}
}

// Two that close the engine each reopen only their own close: a run
// waits until both have reopened. The test above failed in CI on 13
// November 2026 when one reopen lifted the other's close.
func TestOneReopenNeverLiftsAnothersClose(t *testing.T) {
	e, _ := newEngine(t, &finder{profiles: invented})
	first, ok1 := e.CloseIfIdle()
	second, ok2 := e.CloseIfIdle()
	if !ok1 || !ok2 {
		t.Fatal("an idle engine should close")
	}
	first()
	first()
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("start with one close left: %v, want ErrClosed", err)
	}
	second()
	if _, started, err := e.Start("manual", 0); err != nil || !started {
		t.Fatalf("start once both reopened: %v %v", started, err)
	}
	e.Wait()
}
