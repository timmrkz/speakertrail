// Package engine runs profile searches: it asks Exa for LinkedIn profiles
// of a kind of person in a place, like "Personal Trainer Düsseldorf",
// keeps the people who live in NRW, were active on LinkedIn lately and run
// their own business, and scores them by the fit rubric. The engine never
// opens LinkedIn: what a profile says comes from Exa's own index.
//
// One run goes at a time, started by hand. It takes the searches that are
// due, a few side by side, and can be stopped at any moment. What it did
// not get to waits for the next run.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/rubric"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
)

// Provider is the budget name of the one search provider runs use.
const Provider = "exa"

// SideBySide is how many searches a run asks at the same time.
const SideBySide = 4

// Finder finds LinkedIn profiles that match a query. search.Exa is one,
// tests have their own.
type Finder interface {
	People(ctx context.Context, query string, results int) ([]search.Profile, error)
}

// Errors a run can start with.
var (
	ErrNoKey    = errors.New("add the Exa key in Settings first, runs ask Exa for the profiles")
	ErrNoBudget = errors.New("this month's Exa searches are used up. Raise exa_monthly_searches in Settings, or wait for next month")
	ErrNothing  = errors.New("no search is due. Add one in Searches, or resume a paused one")
)

// Engine runs profile searches against a store.
type Engine struct {
	Store *store.Store
	// Finder is the search provider, nil while there is no key.
	Finder func() Finder
	Log    *slog.Logger

	mu    sync.Mutex
	going *going
}

// going is the run that goes now.
type going struct {
	id     int64
	cancel context.CancelFunc
	done   chan struct{}
	// running are the searches being asked now, by id, with when they
	// started.
	running map[int64]time.Time
}

func (e *Engine) log() *slog.Logger {
	if e.Log == nil {
		return slog.Default()
	}
	return e.Log
}

// Recover ends the runs the app closed under, once, when it starts.
func (e *Engine) Recover() error {
	return e.Store.Update(func(d *store.Data) error {
		for _, r := range d.Runs {
			if r.FinishedAt.IsZero() {
				r.Ended, r.FinishedAt = "interrupted", r.StartedAt
				for _, c := range r.Checks {
					if c.At.After(r.FinishedAt) {
						r.FinishedAt = c.At
					}
				}
			}
		}
		return nil
	})
}

// Start begins a run over the due searches, or over the one search named
// for Check now. It returns the run's id, and false with the going run's
// id when one goes already.
func (e *Engine) Start(kind string, only int64) (int64, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.going != nil {
		return e.going.id, false, nil
	}
	if e.Finder == nil || e.Finder() == nil {
		return 0, false, ErrNoKey
	}
	now := e.Store.Now()
	var run *store.Run
	err := e.Store.Update(func(d *store.Data) error {
		var ids []int64
		if only != 0 {
			if d.Search(only) == nil {
				return fmt.Errorf("no search %d", only)
			}
			ids = []int64{only}
		} else {
			for _, s := range d.Due(now, d.Int("profile_searches_per_run")) {
				ids = append(ids, s.ID)
			}
		}
		if len(ids) == 0 {
			return ErrNothing
		}
		if d.Used(Provider, now) >= d.Int(Provider+"_monthly_searches") {
			return ErrNoBudget
		}
		// Everyone is scored again by the rubric as of now: an age turns 30,
		// an activity grows old, the rubric itself changes.
		for _, p := range d.People {
			score(p, now)
		}
		run = &store.Run{ID: d.NewID(), Kind: kind, StartedAt: now, Searches: ids, Checks: []store.Check{}}
		d.Runs = append(d.Runs, run)
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &going{id: run.ID, cancel: cancel, done: make(chan struct{}), running: map[int64]time.Time{}}
	e.going = g
	go e.run(ctx, g, run.Searches)
	e.log().Info("a run started", "run", run.ID, "searches", len(run.Searches))
	return run.ID, true, nil
}

// Stop ends the going run. What it did not get to waits for the next
// run. It returns once the run has ended.
func (e *Engine) Stop(id int64) bool {
	e.mu.Lock()
	g := e.going
	e.mu.Unlock()
	if g == nil || g.id != id {
		return false
	}
	g.cancel()
	<-g.done
	return true
}

// Wait returns once no run goes. Tests and the app's closing use it.
func (e *Engine) Wait() {
	e.mu.Lock()
	g := e.going
	e.mu.Unlock()
	if g != nil {
		<-g.done
	}
}

// Going is the id of the going run, and what it asks now with since
// when, or zero.
func (e *Engine) Going() (int64, map[int64]time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.going == nil {
		return 0, nil
	}
	now := make(map[int64]time.Time, len(e.going.running))
	for id, at := range e.going.running {
		now[id] = at
	}
	return e.going.id, now
}

func (e *Engine) run(ctx context.Context, g *going, ids []int64) {
	defer close(g.done)
	work := make(chan int64)
	var wg sync.WaitGroup
	for range SideBySide {
		wg.Go(func() {
			for id := range work {
				e.check(ctx, g, id)
			}
		})
	}
feed:
	for _, id := range ids {
		select {
		case work <- id:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	ended := ""
	if ctx.Err() != nil {
		ended = "stopped"
	}
	if err := e.Store.Update(func(d *store.Data) error {
		if r := d.Run(g.id); r != nil {
			r.FinishedAt, r.Ended = e.Store.Now(), ended
		}
		return nil
	}); err != nil {
		e.log().Error("the end of a run was not saved", "run", g.id, "error", err)
	}
	e.mu.Lock()
	e.going = nil
	e.mu.Unlock()
	e.log().Info("a run ended", "run", g.id, "ended", cmpOr(ended, "finished"))
}

// check asks one search and keeps whom it finds.
func (e *Engine) check(ctx context.Context, g *going, id int64) {
	if ctx.Err() != nil {
		return
	}
	start := e.Store.Now()
	var query string
	var results int
	var active time.Duration
	reserved := false
	err := e.Store.Update(func(d *store.Data) error {
		s := d.Search(id)
		if s == nil {
			return nil
		}
		query, results, active = s.Query, d.Int("profile_search_results"), d.Days("profile_active_days")
		reserved = d.Reserve(Provider, start)
		return nil
	})
	if err != nil || query == "" {
		e.log().Error("a search could not start", "search", id, "error", err)
		return
	}
	if !reserved {
		e.log().Info("a search waits, no Exa searches left this month", "search", id)
		return
	}
	e.mu.Lock()
	g.running[id] = start
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(g.running, id)
		e.mu.Unlock()
	}()

	finder := e.Finder()
	var profiles []search.Profile
	if finder == nil {
		err = ErrNoKey
	} else {
		profiles, err = finder.People(ctx, query, results)
	}
	if ctx.Err() != nil {
		// Stopped. The search is due again in the next run, and the call
		// stays counted, because Exa may bill it.
		return
	}
	c := store.Check{SearchID: id, At: e.Store.Now()}
	c.DurationMS = c.At.Sub(start).Milliseconds()
	if err != nil {
		c.Error = err.Error()
		e.log().Warn("a search failed", "search", id, "error", err)
	}
	err = e.Store.Update(func(d *store.Data) error {
		s := d.Search(id)
		if s == nil {
			return nil
		}
		seen := map[string]bool{}
		for _, prof := range profiles {
			profile := linkedIn(prof.URL)
			if profile == "" || seen[profile] || !Guest(prof, c.At, c.At.Add(-active)) {
				continue
			}
			seen[profile] = true
			city, _ := extract.PlaceInNRW(prof.Location)
			born, reason, roles := career(prof.Career)
			p, isNew := d.AddFound(store.Found{
				URL: profile, Handle: handleOf(profile), Name: prof.Name, Headline: prof.Headline, City: city,
				BornAbout: born, AgeReason: reason, Roles: roles, LastActive: prof.LastActive,
			}, s, g.id, c.At)
			score(p, c.At)
			c.PeopleFound++
			if isNew {
				c.PeopleNew++
			}
		}
		d.Ran(s, c)
		if r := d.Run(g.id); r != nil {
			r.Checks = append(r.Checks, c)
		}
		return nil
	})
	if err != nil {
		e.log().Error("what a search found was not saved", "search", id, "error", err)
	}
}

// Guest tells whether a profile shows a possible guest: in NRW, active on
// LinkedIn since a time, running their own business, and not ruled out.
func Guest(prof search.Profile, now, activeSince time.Time) bool {
	if _, inNRW := extract.PlaceInNRW(prof.Location); !inNRW {
		return false
	}
	if prof.LastActive.Before(activeSince) {
		return false
	}
	_, _, roles := career(prof.Career)
	ok, _ := rubric.Guest(rubric.Signals(rubric.Person{Name: prof.Name, Headline: prof.Headline, CurrentRoles: roles, Now: now}))
	return ok
}

// maxRoles is how many roles held now are kept of a profile.
const maxRoles = 3

// career reads what a profile's jobs and education show: the likely year
// of birth with the step that shows it, and the roles held now. The list
// itself is not kept.
func career(steps []search.Step) (born int, reason, roles string) {
	in := make([]rubric.Step, 0, len(steps))
	var now []string
	for _, s := range steps {
		in = append(in, rubric.Step{Education: s.Education, Title: s.Title, Org: s.Org, From: s.From, To: s.To})
		if s.Education || s.To != 0 || s.Title == "" || len(now) == maxRoles {
			continue
		}
		role := s.Title
		if s.Org != "" {
			role += ", " + s.Org
		}
		now = append(now, role)
	}
	born, reason = rubric.BornAbout(in)
	return born, reason, strings.Join(now, " · ")
}

// score runs the fit rubric over a person as of now.
func score(p *store.Person, now time.Time) {
	if p.LinkedIn.Review == "rejected" {
		p.Signals, p.Score, p.Rubric = []store.Signal{}, 0, rubric.Version
		return
	}
	sigs := rubric.Signals(rubric.Person{
		Name: p.Name, Headline: p.Headline, CurrentRoles: p.LinkedIn.Roles,
		BornAbout: p.LinkedIn.BornAbout, AgeReason: p.LinkedIn.AgeReason, LastActive: p.LinkedIn.LastActive, Now: now,
	})
	p.Signals = make([]store.Signal, 0, len(sigs))
	for _, s := range sigs {
		label := s.Key
		if d := rubric.Lookup(s.Key); d != nil {
			label = d.Label
		}
		p.Signals = append(p.Signals, store.Signal{Key: s.Key, Label: label, For: s.For, Passage: s.Passage, FoundIn: s.Where})
	}
	p.Score, p.Rubric = rubric.Score(sigs), rubric.Version
}

// linkedIn gives a LinkedIn profile's link in one spelling, or nothing
// for any other link.
func linkedIn(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com") {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasPrefix(path, "/in/") {
		return ""
	}
	return "https://www.linkedin.com" + path
}

// handleOf is the last part of a profile's link.
func handleOf(profile string) string {
	return profile[strings.LastIndex(profile, "/")+1:]
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
