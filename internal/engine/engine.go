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

	"github.com/timmrkz/speakertrail/internal/place"
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
	ErrClosed   = errors.New("the app is relaunching into a new build. Start the run again once it is back")
	// ErrUnread is a key that is saved and could not be read, like one
	// macOS kept back because its password box was closed. The error a
	// run starts with says why in its own words.
	ErrUnread = errors.New("the Exa key could not be read")
)

// unread is a Finder's failure other than ErrNoKey, said in its own
// words and matched as ErrUnread.
type unread struct{ err error }

func (u unread) Error() string        { return u.err.Error() }
func (u unread) Unwrap() error        { return u.err }
func (u unread) Is(target error) bool { return target == ErrUnread }

// Engine runs profile searches against a store.
type Engine struct {
	Store *store.Store
	// Finder is the search provider. It fails with ErrNoKey while no key
	// is saved, and with why when the key could not be read, like a
	// password box closed on the Mac. Each ask tries again.
	Finder func() (Finder, error)
	Log    *slog.Logger
	// Act is what the person did last, the id the app's log ties lines
	// to, or nil. A run keeps the act that started it on every line it
	// writes, however long it goes, the way a job keeps it in
	// FrameFairy. See docs/LOGGING.md.
	Act func() string

	mu    sync.Mutex
	going *going
	// closed counts those who closed the engine and have not reopened it:
	// Relaunch, while it puts a new build in place, so no run starts. A
	// count, not a flag, so one reopen never lifts another's close.
	closed int
}

// going is the run that goes now.
type going struct {
	id     int64
	cancel context.CancelFunc
	done   chan struct{}
	// running are the searches being asked now, by id, with when they
	// started.
	running map[int64]time.Time
	// log is the engine's log with the act that started the run.
	log *slog.Logger
}

func (e *Engine) log() *slog.Logger {
	if e.Log == nil {
		return slog.Default()
	}
	return e.Log
}

// Recover ends the runs the app closed under, once, when it starts, and
// scores again everyone a rubric older than this one scored, so a change
// to it reaches the people found before.
func (e *Engine) Recover() error {
	if err := e.rescore(); err != nil {
		return err
	}
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

// CloseIfIdle says whether no run goes, and when none does, starts none
// until reopen is called. Relaunch asks it, so a run is never stopped half
// way by the quit into a new build, and none starts between the look and
// the quit.
func (e *Engine) CloseIfIdle() (reopen func(), ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.going != nil {
		return func() {}, false
	}
	e.closed++
	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Lock()
			e.closed--
			e.mu.Unlock()
		})
	}, true
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
	if e.closed > 0 {
		return 0, false, ErrClosed
	}
	if e.Finder == nil {
		return 0, false, ErrNoKey
	}
	if _, err := e.Finder(); errors.Is(err, ErrNoKey) {
		return 0, false, ErrNoKey
	} else if err != nil {
		return 0, false, unread{err}
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
	g := &going{id: run.ID, cancel: cancel, done: make(chan struct{}), running: map[int64]time.Time{}, log: e.log()}
	if e.Act != nil {
		if act := e.Act(); act != "" {
			g.log = g.log.With("act", act)
		}
	}
	e.going = g
	go e.run(ctx, g, run.Searches)
	g.log.Info("a run started", "run", run.ID, "searches", len(run.Searches))
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
		g.log.Error("the end of a run was not saved", "run", g.id, "error", err)
	}
	e.mu.Lock()
	e.going = nil
	e.mu.Unlock()
	g.log.Info("a run ended", "run", g.id, "ended", cmpOr(ended, "finished"))
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
		g.log.Error("a search could not start", "run", g.id, "search", id, "error", err)
		return
	}
	if !reserved {
		g.log.Info("a search waits, no Exa searches left this month", "run", g.id, "search", id)
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

	var profiles []search.Profile
	finder, err := e.Finder()
	if err == nil {
		profiles, err = finder.People(ctx, query, results)
	}
	if ctx.Err() != nil {
		// Stopped. The search is due again in the next run, and the call
		// stays counted, because Exa may bill it.
		g.log.Debug("a search was stopped", "run", g.id, "search", id)
		return
	}
	c := store.Check{SearchID: id, At: e.Store.Now()}
	c.DurationMS = c.At.Sub(start).Milliseconds()
	if err != nil {
		c.Error = err.Error()
		g.log.Warn("a search failed", "run", g.id, "search", id, "ms", c.DurationMS, "error", err)
	}
	err = e.Store.Update(func(d *store.Data) error {
		s := d.Search(id)
		if s == nil {
			return nil
		}
		seen := map[string]bool{}
		exclusions := d.Phrases()
		for _, prof := range profiles {
			profile := linkedIn(prof.URL)
			if profile == "" || seen[profile] || !Guest(prof, c.At, c.At.Add(-active), exclusions) {
				continue
			}
			seen[profile] = true
			city, _ := place.PlaceInNRW(prof.Location)
			born, reason, roles := careerOf(prof, c.At)
			p, isNew := d.AddFound(store.Found{
				URL: profile, Handle: handleOf(profile), Name: prof.Name, Headline: prof.Headline, City: city,
				BornAbout: born, AgeReason: reason, Roles: roles, LastActive: prof.LastActive, Text: prof.Text,
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
		g.log.Error("what a search found was not saved", "run", g.id, "search", id, "error", err)
		return
	}
	// Every check is a line, by ids and counts and never by whom it
	// found, which is personal data.
	if c.Error == "" {
		g.log.Debug("a search was asked", "run", g.id, "search", id, "ms", c.DurationMS,
			"profiles", len(profiles), "found", c.PeopleFound, "new", c.PeopleNew)
	}
}

// Guest tells whether a profile shows a possible guest: in NRW, active on
// LinkedIn since a time, running their own business, and neither ruled out
// nor left out by one of Tim's exclusions.
func Guest(prof search.Profile, now, activeSince time.Time, exclusions []string) bool {
	if _, inNRW := place.PlaceInNRW(prof.Location); !inNRW {
		return false
	}
	if prof.LastActive.Before(activeSince) {
		return false
	}
	_, _, roles := career(prof.Career)
	if _, out := rubric.Excluded(prof.Headline+" "+roles, exclusions); out {
		return false
	}
	ok, _ := rubric.Guest(rubric.Signals(rubric.Person{Name: prof.Name, Headline: prof.Headline, CurrentRoles: roles, Now: now}))
	return ok
}

// maxRoles is how many roles held now are kept of a profile.
const maxRoles = 3

// careerOf is career, except that an age the headline gives, like "19
// year old student", beats the guess from the years on the profile. Tim
// saw "19 year old student" shown as about 25 on 10 November 2026.
func careerOf(prof search.Profile, now time.Time) (born int, reason, roles string) {
	born, reason, roles = career(prof.Career)
	if age := rubric.StatedAge(prof.Headline); age > 0 {
		born, reason = now.Year()-age, fmt.Sprintf("Headline says %d in %d", age, now.Year())
	}
	return born, reason, roles
}

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

// rescore scores again everyone an older rubric scored. The rubric takes
// most of a millisecond a person, so it reads them, scores them outside
// the store's lock, and only then writes the signals back, to everyone
// nothing changed meanwhile.
func (e *Engine) rescore() error {
	start, now := time.Now(), e.Store.Now()
	type stale struct {
		in       rubric.Person
		rejected bool
	}
	todo := map[int64]stale{}
	e.Store.Read(func(d *store.Data) {
		for _, p := range d.People {
			if p.Rubric != rubric.Version {
				todo[p.ID] = stale{rubricPerson(p, now), p.Rejected()}
			}
		}
	})
	if len(todo) == 0 {
		return nil
	}
	type scored struct {
		sigs  []store.Signal
		score int
	}
	done := make(map[int64]scored, len(todo))
	for id, s := range todo {
		if s.rejected {
			done[id] = scored{[]store.Signal{}, 0}
			continue
		}
		sigs, n := signalsOf(s.in)
		done[id] = scored{sigs, n}
	}
	err := e.Store.Update(func(d *store.Data) error {
		for _, p := range d.People {
			s, ok := done[p.ID]
			if ok && p.Rubric != rubric.Version && p.Rejected() == todo[p.ID].rejected && sameFacts(rubricPerson(p, now), todo[p.ID].in) {
				p.Signals, p.Score, p.Rubric = s.sigs, s.score, rubric.Version
			}
		}
		return nil
	})
	e.log().Info("people scored again by a newer rubric", "people", len(done), "rubric", rubric.Version, "ms", time.Since(start).Milliseconds())
	return err
}

// rubricPerson is what the rubric reads of a person.
func rubricPerson(p *store.Person, now time.Time) rubric.Person {
	return rubric.Person{
		Name: p.Name, Headline: p.Headline, CurrentRoles: p.LinkedIn.Roles,
		BornAbout: p.LinkedIn.BornAbout, AgeReason: p.LinkedIn.AgeReason, LastActive: p.LinkedIn.LastActive, Now: now,
	}
}

// sameFacts tells whether the rubric reads the same of two people.
func sameFacts(a, b rubric.Person) bool {
	return a.Name == b.Name && a.Headline == b.Headline && a.CurrentRoles == b.CurrentRoles &&
		a.BornAbout == b.BornAbout && a.AgeReason == b.AgeReason && a.LastActive.Equal(b.LastActive)
}

// signalsOf runs the fit rubric over what it reads of a person.
func signalsOf(in rubric.Person) ([]store.Signal, int) {
	sigs := rubric.Signals(in)
	out := make([]store.Signal, 0, len(sigs))
	for _, s := range sigs {
		label := s.Key
		if d := rubric.Lookup(s.Key); d != nil {
			label = d.Label
		}
		out = append(out, store.Signal{Key: s.Key, Label: label, For: s.For, Passage: s.Passage, FoundIn: s.Where})
	}
	return out, rubric.Score(sigs)
}

// score runs the fit rubric over a person as of now.
func score(p *store.Person, now time.Time) {
	if p.Rejected() {
		p.Signals, p.Score, p.Rubric = []store.Signal{}, 0, rubric.Version
		return
	}
	p.Signals, p.Score = signalsOf(rubricPerson(p, now))
	p.Rubric = rubric.Version
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
