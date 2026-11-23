package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/rubric"
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
	// read are the profiles Profile reads, by link. A link not in it
	// is not in the index.
	read  map[string]search.Profile
	links []string
	// readHold, when set, holds Profile in place of hold, so a read and a
	// search can be let go one apart from the other.
	readHold chan struct{}
	// byName are the profiles ProfileByName finds, by link, and named the
	// links it was asked for.
	byName map[string]search.Profile
	named  []string
}

func (f *finder) ProfileByName(ctx context.Context, link string) (search.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.named = append(f.named, link)
	if prof, ok := f.byName[link]; ok {
		return prof, nil
	}
	return search.Profile{}, search.ErrNotIndexed
}

func (f *finder) Profile(ctx context.Context, link string) (search.Profile, error) {
	f.mu.Lock()
	f.links = append(f.links, link)
	prof, ok := f.read[link]
	f.mu.Unlock()
	if f.started != nil {
		f.started <- link
	}
	hold := f.hold
	if f.readHold != nil {
		hold = f.readHold
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return search.Profile{}, ctx.Err()
		}
	}
	if !ok {
		return search.Profile{}, fmt.Errorf("%w at %s", search.ErrNotIndexed, link)
	}
	return prof, f.err
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
	e := &Engine{Store: st, Finder: func() (Finder, error) {
		if f == nil {
			return nil, ErrNoKey
		}
		return f, nil
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

// A key the keychain kept back, a password box closed, stops the run with
// why, not as a missing key, and the next start asks for it again.
func TestARunWhoseKeyWasKeptBackSaysWhyAndStartsNextTime(t *testing.T) {
	e, st := newEngine(t, &finder{profiles: invented})
	keptBack := errors.New("the password box was closed, so macOS kept the Exa key back")
	var asked atomic.Int32
	found := e.Finder
	e.Finder = func() (Finder, error) {
		if asked.Add(1) == 1 {
			return nil, keptBack
		}
		return found()
	}
	if _, _, err := e.Start("manual", 0); !errors.Is(err, ErrUnread) || !errors.Is(err, keptBack) || err.Error() != keptBack.Error() {
		t.Fatalf("err = %v, want it kept back, as ErrUnread", err)
	}
	id, started, err := e.Start("manual", 0)
	if err != nil || !started {
		t.Fatalf("the second start: %v, %v", started, err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		if r := d.Run(id); r == nil || r.FinishedAt.IsZero() || len(r.Checks) != 1 || r.Checks[0].Error != "" {
			t.Errorf("run %+v", r)
		}
	})
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

// A change to the rubric reaches the people found before it when the app
// starts, not only once a search finds them again.
func TestRecoverScoresAgain(t *testing.T) {
	e, st := newEngine(t, &finder{})
	_ = st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Gründer Köln", 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/mara-kowal", Name: "Mara Kowal",
			Headline: "Founder of Probe AI", Roles: "Founder, Probe AI · Projektmanagerin, Muster Versicherung AG"}, s, 0, t0)
		p.Signals, p.Score, p.Rubric = []store.Signal{{Key: "owner_operator", For: true}}, 1, rubric.Version-1
		return nil
	})
	if err := e.Recover(); err != nil {
		t.Fatal(err)
	}
	st.Read(func(d *store.Data) {
		p := d.People[0]
		var keys []string
		for _, s := range p.Signals {
			keys = append(keys, s.Key)
		}
		if p.Rubric != rubric.Version || !slices.Contains(keys, "employed") {
			t.Errorf("rubric %d, signals %v", p.Rubric, keys)
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

// lines is a log written as JSON, read back a line each, safe to write
// from the run's goroutines while a test reads it after.
type lines struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lines) all(t *testing.T) []map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, b := range bytes.Split(bytes.TrimSpace(l.buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("a line is not whole JSON: %v\n%s", err, b)
		}
		out = append(out, m)
	}
	return out
}

// Every check of a run is a line of the log with the run, the search,
// how long it took and what it found, and a failed one with its error
// too, while several are asked side by side. Each keeps the act that
// started the run, though the person goes on to others. The lines name
// nobody: no name, headline or profile link of a person found.
func TestEveryCheckIsALineOfTheLog(t *testing.T) {
	e, st := newEngine(t, &finder{profiles: invented})
	if err := st.Update(func(d *store.Data) error {
		for _, q := range []string{"Yogalehrerin Bonn", "Gründer Essen", "Trainerin Münster", "Coach Aachen", "Fotografin Bielefeld"} {
			if _, err := d.AddSearch(q, 0, t0); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out lines
	e.Log = slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var acting atomic.Value
	acting.Store("a5")
	e.Act = func() string { return acting.Load().(string) }
	id, _, err := e.Start("manual", 0)
	acting.Store("a6")
	if err != nil {
		t.Fatal(err)
	}
	e.Wait()

	asked := 0
	for _, l := range out.all(t) {
		if l["msg"] != "a search was asked" {
			continue
		}
		asked++
		if l["run"] != float64(id) || l["act"] != "a5" || l["search"] == nil || l["ms"] == nil || l["found"] != float64(1) || l["new"] == nil || l["profiles"] != float64(len(invented)) {
			t.Errorf("a check's line is %v", l)
		}
	}
	if asked != 6 {
		t.Errorf("%d checks were logged, want 6", asked)
	}
	text := out.buf.String()
	for _, p := range invented {
		if strings.Contains(text, p.Name) || strings.Contains(text, p.Headline) || strings.Contains(text, "linkedin.com") {
			t.Fatalf("the log names a person: %s", text)
		}
	}

	f, _ := newEngine(t, &finder{err: errors.New("exa answered 500")})
	var failed lines
	f.Log = slog.New(slog.NewJSONHandler(&failed, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fid, _, err := f.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Wait()
	found := false
	for _, l := range failed.all(t) {
		if l["msg"] == "a search failed" {
			found = true
			if l["run"] != float64(fid) || l["ms"] == nil || l["error"] != "exa answered 500" {
				t.Errorf("a failed check's line is %v", l)
			}
		}
	}
	if !found {
		t.Error("a failed check wrote no line")
	}
}

// A profile the invented index holds, of someone Tim connected with.
var ida = search.Profile{
	URL: "https://www.linkedin.com/in/ida-probe-7f", Name: "Ida Probe", Headline: "Inhaberin, Probe Studio",
	Location: "Essen, Nordrhein-Westfalen, Deutschland", LastActive: t0.AddDate(0, -1, 0),
	Career: steps("Inhaberin", "Probe Studio", 2022, "Universität Duisburg-Essen", 2018), Text: "Ida Probe\nInhaberin, Probe Studio",
}

func TestLinks(t *testing.T) {
	text := `Leute vom Gründerfrühstück:
https://www.linkedin.com/in/ida-probe-7f/
https://de.linkedin.com/in/ole-muster?utm_source=share, (linkedin.com/in/ida-probe-7f)
www.linkedin.com/in/pia-beispiel https://www.linkedin.com/company/probe-gmbh/ linkedin.com/in/ https://example.org`
	profiles, other := Links(text)
	want := []string{"https://www.linkedin.com/in/ida-probe-7f", "https://www.linkedin.com/in/ole-muster", "https://www.linkedin.com/in/pia-beispiel"}
	if strings.Join(profiles, " ") != strings.Join(want, " ") || other != 3 {
		t.Errorf("Links = %q and %d others, want %q and 3", profiles, other, want)
	}
	if profiles, other := Links("Yogalehrerin Köln"); len(profiles) != 0 || other != 0 {
		t.Errorf("words alone gave %q and %d", profiles, other)
	}
}

// Links Tim adds are people at once, connected and named by their handle,
// and a run of their own reads their profiles and asks no search. A
// profile Exa's index does not hold keeps the handle and says why.
func TestAddReadsTheProfilesOfTheLinks(t *testing.T) {
	f := &finder{read: map[string]search.Profile{ida.URL: ida}}
	e, st := newEngine(t, f)
	out, err := e.Add(ida.URL + "/ linkedin.com/in/neu-hier-3 https://www.linkedin.com/company/probe-gmbh")
	if err != nil || out.Run == 0 || len(out.New) != 2 || len(out.Here) != 0 || out.Other != 1 {
		t.Fatalf("Add = %+v, %v", out, err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		p := d.PersonByProfile(ida.URL)
		if p == nil || p.Name != "Ida Probe" || !p.Connected || p.ToRead || p.City != "Essen" || p.LinkedIn.Text == "" || p.LinkedIn.BornAbout == 0 || p.Evidence != store.AddedByHand {
			t.Errorf("read %+v", p)
		}
		q := d.PersonByProfile("https://www.linkedin.com/in/neu-hier-3")
		if q == nil || q.Name != "neu-hier-3" || !q.Connected || q.ToRead || q.ReadError != NotIndexed {
			t.Errorf("not in the index %+v", q)
		}
		r := d.Run(out.Run)
		if r == nil || r.Kind != "add" || r.FinishedAt.IsZero() || len(r.Links) != 2 || len(r.Reads) != 2 || len(r.Searches) != 0 {
			t.Errorf("run %+v", r)
		}
		for _, read := range r.Reads {
			if strings.Contains(read.Error, "linkedin") {
				t.Errorf("a read's error names the link: %q", read.Error)
			}
		}
		// One for each link, and one more asking for the one not in the
		// index by name.
		if d.Used(Provider, t0) != 3 {
			t.Errorf("used %d Exa calls, want 3", d.Used(Provider, t0))
		}
	})
	if len(f.queries) != 0 || fmt.Sprint(f.named) != "[https://www.linkedin.com/in/neu-hier-3]" {
		t.Errorf("adding links asked the searches %q, by name %q", f.queries, f.named)
	}
	// The same link again only marks them connected, nobody is added.
	_ = st.Update(func(d *store.Data) error { d.PersonByProfile(ida.URL).Connect(false, t0); return nil })
	again, err := e.Add("https://www.linkedin.com/in/ida-probe-7f")
	if err != nil || len(again.New) != 0 || len(again.Here) != 1 || again.Run != 0 {
		t.Errorf("again %+v, %v", again, err)
	}
	st.Read(func(d *store.Data) {
		if len(d.People) != 2 || !d.PersonByProfile(ida.URL).Connected {
			t.Errorf("people after adding the same link: %d", len(d.People))
		}
	})
	if _, err := e.Add("Yogalehrerin Köln"); !errors.Is(err, ErrNoLinks) {
		t.Errorf("words alone: %v, want ErrNoLinks", err)
	}
}

// A profile Exa holds under another link than Tim's is found by the name
// in the link, and read as if under his. Without a search left for that
// this month, the person waits for the next run.
func TestAProfileNotUnderItsLinkIsAskedForByName(t *testing.T) {
	link := "https://www.linkedin.com/in/jule-probe-75"
	jule := search.Profile{URL: "https://de.linkedin.com/in/jule-probe-75", Name: "Jule Probe", Headline: "Inhaberin, Probe Atelier", Location: "Münster, Germany", Text: "# Jule Probe\n\nInhaberin, Probe Atelier"}
	f := &finder{byName: map[string]search.Profile{link: jule}}
	e, st := newEngine(t, f)
	if _, err := e.Add(link); err != nil {
		t.Fatal(err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		p := d.PersonByProfile(link)
		if p == nil || p.Name != "Jule Probe" || p.ToRead || p.ReadError != "" || p.City != "Münster" || p.LinkedIn.URL != link || p.LinkedIn.Text == "" {
			t.Errorf("read %+v", p)
		}
	})
	// The month has one search left, enough for the link but not the name.
	_ = st.Update(func(d *store.Data) error {
		return d.SetSetting(Provider+"_monthly_searches", json.RawMessage(strconv.Itoa(d.Used(Provider, t0)+1)), t0)
	})
	other := "https://www.linkedin.com/in/olaf-probe-78"
	if _, err := e.Add(other); err != nil {
		t.Fatal(err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		p := d.PersonByProfile(other)
		if p == nil || !p.ToRead || p.ReadError != "" {
			t.Errorf("without a search left %+v", p)
		}
	})
	if len(f.named) != 1 {
		t.Errorf("asked by name for %q", f.named)
	}
}

// Without a key the people are added all the same, and the next run
// reads them.
func TestAddWithoutAKeyLeavesTheProfilesForTheNextRun(t *testing.T) {
	e, st := newEngine(t, nil)
	out, err := e.Add(ida.URL)
	if !errors.Is(err, ErrNoKey) || len(out.New) != 1 || out.Run != 0 {
		t.Fatalf("Add = %+v, %v", out, err)
	}
	st.Read(func(d *store.Data) {
		if p := d.PersonByProfile(ida.URL); p == nil || !p.ToRead || !p.Connected {
			t.Errorf("added %+v", p)
		}
	})
}

// Links added while a run goes join it, read before the searches it has
// left. Stopped, the profiles not read wait for the next run, which reads
// them.
func TestLinksJoinTheRunGoingAndStopLeavesThemForTheNext(t *testing.T) {
	f := &finder{hold: make(chan struct{}), started: make(chan string, 64), read: map[string]search.Profile{ida.URL: ida}}
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
	out, err := e.Add(ida.URL + " linkedin.com/in/neu-hier-3")
	if err != nil || out.Run != id || len(out.New) != 2 {
		t.Fatalf("Add = %+v, %v, want it to join run %d", out, err, id)
	}
	if reading, waiting := e.Reading(); len(reading) != 0 || len(waiting) != 2 {
		t.Errorf("reading %v, waiting %v, want both waiting", reading, waiting)
	}
	if !e.Stop(id) {
		t.Fatal("Stop found no run")
	}
	st.Read(func(d *store.Data) {
		r := d.Run(id)
		if len(r.Links) != 2 || len(r.Reads) != 0 {
			t.Errorf("stopped run %+v", r)
		}
		for _, p := range d.People {
			if !p.ToRead {
				t.Errorf("%s was not left for the next run", p.Name)
			}
		}
	})
	close(f.hold)
	next, _, err := e.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	e.Wait()
	st.Read(func(d *store.Data) {
		if r := d.Run(next); len(r.Links) != 2 || len(r.Reads) != 2 {
			t.Errorf("the next run %+v", r)
		}
		if p := d.PersonByProfile(ida.URL); p.ToRead || p.Name != "Ida Probe" {
			t.Errorf("after the next run %+v", p)
		}
	})
}

// The window adds links, starts, stops and watches runs from several
// goroutines at once.
func TestAddStartStopAndReadingFromSeveralGoroutines(t *testing.T) {
	f := &finder{profiles: invented, read: map[string]search.Profile{ida.URL: ida}}
	e, st := newEngine(t, f)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			link := fmt.Sprintf("https://www.linkedin.com/in/probe-%d", i%4)
			if _, err := e.Add(link + " " + ida.URL); err != nil {
				t.Errorf("Add: %v", err)
			}
		})
		// A link added and taken out again by Undo, meanwhile.
		wg.Go(func() {
			out, err := e.Add(fmt.Sprintf("https://www.linkedin.com/in/undone-%d", i))
			if err != nil {
				t.Errorf("Add: %v", err)
			}
			for _, id := range out.New {
				if err := e.Remove(id); err != nil {
					t.Errorf("Remove: %v", err)
				}
			}
		})
		wg.Go(func() {
			if id, _, err := e.Start("manual", 0); err == nil && i%3 == 0 {
				e.Stop(id)
			}
		})
		wg.Go(func() { e.Reading() })
		wg.Go(func() { e.Going() })
	}
	wg.Wait()
	e.Wait()
	st.Read(func(d *store.Data) {
		seen := map[string]bool{}
		for _, p := range d.People {
			if p.Evidence != store.AddedByHand {
				continue
			}
			if seen[p.LinkedIn.URL] {
				t.Errorf("%s was added twice", p.LinkedIn.URL)
			}
			seen[p.LinkedIn.URL] = true
		}
		if len(seen) != 5 {
			t.Errorf("%d people added from links, want 5: Ida and four probes", len(seen))
		}
	})
}

// The same link dropped again and again while its profile is taken up
// is read once: each read is a search of the month's budget.
func TestALinkAddedFromSeveralGoroutinesIsReadOnce(t *testing.T) {
	for range 20 {
		f := &finder{read: map[string]search.Profile{ida.URL: ida}}
		e, st := newEngine(t, f)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 10 {
					if _, err := e.Add(ida.URL); err != nil {
						t.Error(err)
					}
				}
			})
		}
		wg.Wait()
		e.Wait()
		f.mu.Lock()
		reads := len(f.links)
		f.mu.Unlock()
		if reads != 1 {
			t.Fatalf("the profile was read %d times", reads)
		}
		st.Read(func(d *store.Data) {
			if n := d.Used(Provider, t0); n != 1 {
				t.Errorf("%d searches of the budget used, want 1", n)
			}
		})
	}
}

// A profile whose read failed in a run is read again by the next run,
// not by the same one when its link is added again, so the run never
// counts more profiles done than it has.
func TestALinkAddedAgainAfterAFailedReadWaitsForTheNextRun(t *testing.T) {
	f := &finder{hold: make(chan struct{}), readHold: make(chan struct{}), started: make(chan string, 64), read: map[string]search.Profile{ida.URL: ida}, err: errors.New("Exa answered 503")}
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
	if _, err := e.Add(ida.URL); err != nil {
		t.Fatal(err)
	}
	// One search lets go of its worker, which takes the profile next.
	f.hold <- struct{}{}
	for got := <-f.started; got != ida.URL; got = <-f.started {
	}
	// The read fails while the search still runs.
	f.readHold <- struct{}{}
	for {
		var failed bool
		st.Read(func(d *store.Data) { failed = len(d.Run(id).Reads) == 1 })
		if failed {
			break
		}
		runtime.Gosched()
	}
	// Added again, nobody is read by this run, so no run reads for it.
	out, err := e.Add(ida.URL)
	if err != nil || out.Run != 0 || len(out.Here) != 1 {
		t.Fatalf("Add again = %+v, %v", out, err)
	}
	close(f.readHold)
	close(f.hold)
	e.Wait()
	st.Read(func(d *store.Data) {
		r := d.Run(id)
		if len(r.Reads) != 1 || len(r.Links) != 1 {
			t.Errorf("the run read %d profiles of %d", len(r.Reads), len(r.Links))
		}
		if p := d.People[len(d.People)-1]; !p.ToRead {
			t.Errorf("%s is not left for the next run", p.Name)
		}
	})
}

// Undo takes someone added out of the queue of the going run, so their
// profile is never read, and out of the run's count.
func TestRemoveTakesSomeoneAddedOutOfTheRun(t *testing.T) {
	f := &finder{hold: make(chan struct{}), started: make(chan string, 64), read: map[string]search.Profile{ida.URL: ida}}
	e, st := newEngine(t, f)
	id, _, err := e.Start("manual", 0)
	if err != nil {
		t.Fatal(err)
	}
	<-f.started
	out, err := e.Add(ida.URL + " linkedin.com/in/neu-hier-3")
	if err != nil || len(out.New) != 2 {
		t.Fatalf("Add = %+v, %v", out, err)
	}
	if err := e.Remove(out.New[1]); err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(out.New[1]); !errors.Is(err, store.ErrNoPerson) {
		t.Errorf("taken out twice: %v", err)
	}
	close(f.hold)
	e.Wait()
	f.mu.Lock()
	links := slices.Clone(f.links)
	f.mu.Unlock()
	if len(links) != 1 || links[0] != ida.URL {
		t.Errorf("read %v, want only Ida's profile", links)
	}
	st.Read(func(d *store.Data) {
		if r := d.Run(id); len(r.Links) != 1 || len(r.Reads) != 1 {
			t.Errorf("the run had %d links and %d reads", len(r.Links), len(r.Reads))
		}
		if d.Person(out.New[1]) != nil {
			t.Error("the person taken out is still there")
		}
	})
}

// A profile Tim pastes while Exa is still asked for it wins over what Exa
// answers, and pasting from several goroutines while the run reads and
// is asked how far it is leaves one profile and no read of it.
func TestAPasteWhileExaReadsWins(t *testing.T) {
	f := &finder{readHold: make(chan struct{}), started: make(chan string, 64), read: map[string]search.Profile{ida.URL: ida}}
	e, st := newEngine(t, f)
	out, err := e.Add(ida.URL + " linkedin.com/in/neu-hier-3")
	if err != nil {
		t.Fatal(err)
	}
	<-f.started
	<-f.started
	page := "Ida Probe\nGründerin | Probe Werkstatt\nInfo\nIch drechsle."
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, _, err := e.Paste(out.New[0], page, nil); err != nil {
				t.Error(err)
			}
			e.Reading()
		})
	}
	wg.Wait()
	close(f.readHold)
	e.Wait()
	st.Read(func(d *store.Data) {
		p := d.Person(out.New[0])
		if p.Headline != "Gründerin | Probe Werkstatt" || p.ToRead || p.ReadError != "" || !strings.Contains(p.LinkedIn.Text, "drechsle") {
			t.Errorf("after the paste %+v", p)
		}
		r := d.Run(out.Run)
		if len(r.Links) != 1 || len(r.Reads) != 1 {
			t.Errorf("the run counts %d links and %d reads, want the one not pasted", len(r.Links), len(r.Reads))
		}
	})
}
