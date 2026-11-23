// Package engine runs profile searches: it asks Exa for LinkedIn profiles
// of a kind of person in a place, like "Personal Trainer Düsseldorf",
// keeps the people who live in NRW, were active on LinkedIn lately and run
// their own business, and scores them by the fit rubric. The engine never
// opens LinkedIn: what a profile says comes from Exa's own index.
//
// One run goes at a time, started by hand. It takes the searches that are
// due, a few side by side, and can be stopped at any moment. What it did
// not get to waits for the next run.
//
// A run also reads the profiles of people Tim added from their LinkedIn
// links, see Add. Adding links starts a run that reads only those, or
// joins the run going. Profiles are read before searches are asked,
// since Tim waits for them.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
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

// SideBySide is how many searches a run asks, or profiles it reads, at
// the same time.
const SideBySide = 4

// Finder finds LinkedIn profiles that match a query, and reads one
// profile by its link. search.Exa is one, tests have their own.
type Finder interface {
	People(ctx context.Context, query string, results int) ([]search.Profile, error)
	Reader
}

// Reader reads one profile from Exa's index by its link, or by the name
// in its link when the index holds it under another.
type Reader interface {
	Profile(ctx context.Context, link string) (search.Profile, error)
	ProfileByName(ctx context.Context, link string) (search.Profile, error)
}

// ReadProfile reads a profile by its link, and when Exa's index has no
// copy under the link, asks Exa once more by the name in it, since the
// index often holds a profile under another, like de.linkedin.com. That
// is one more search, and reserve takes it from the month's budget, or
// says none is left, which is ErrNoBudget.
func ReadProfile(ctx context.Context, r Reader, link string, reserve func() (bool, error)) (search.Profile, error) {
	prof, err := r.Profile(ctx, link)
	if !errors.Is(err, search.ErrNotIndexed) || ctx.Err() != nil {
		return prof, err
	}
	ok, err := reserve()
	switch {
	case err != nil:
		return search.Profile{}, err
	case !ok:
		return search.Profile{}, ErrNoBudget
	}
	return r.ProfileByName(ctx, link)
}

// Errors a run can start with.
var (
	ErrNoKey    = errors.New("add the Exa key in Settings first, runs ask Exa for the profiles")
	ErrNoBudget = errors.New("this month's Exa searches are used up. Raise exa_monthly_searches in Settings, or wait for next month")
	ErrNothing  = errors.New("no search is due. Add one in Searches, or resume a paused one")
	ErrNoLinks  = errors.New("there is no link to a LinkedIn profile in it, like linkedin.com/in/name")
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

// job is one search to ask, or one person whose profile to read.
type job struct{ search, person int64 }

// going is the run that goes now. What it has still to do and who works
// on it change under Engine.mu.
type going struct {
	id     int64
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	// queue is what the run has still to do, in order. Links Tim adds
	// while it goes join it.
	queue []job
	// workers work through the queue, at most SideBySide. The last to
	// find it empty closes idle and sets ending, and from then on
	// nothing joins the run.
	workers int
	ending  bool
	idle    chan struct{}
	// running are the searches being asked now, by id, with when they
	// started, and reading the people whose profiles are read now.
	running map[int64]time.Time
	reading map[int64]time.Time
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
		var ids, links []int64
		if only != 0 {
			if d.Search(only) == nil {
				return fmt.Errorf("no search %d", only)
			}
			ids = []int64{only}
		} else {
			for _, s := range d.Due(now, d.Int("profile_searches_per_run")) {
				ids = append(ids, s.ID)
			}
			// Profiles a run before did not get to read.
			for _, p := range d.People {
				if p.ToRead {
					links = append(links, p.ID)
				}
			}
		}
		if len(ids) == 0 && len(links) == 0 {
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
		run = &store.Run{ID: d.NewID(), Kind: kind, StartedAt: now, Searches: ids, Checks: []store.Check{}, Links: links}
		d.Runs = append(d.Runs, run)
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	e.launch(run)
	return run.ID, true, nil
}

// launch starts the work of a run saved a moment ago. Engine.mu is held.
func (e *Engine) launch(run *store.Run) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &going{id: run.ID, ctx: ctx, cancel: cancel, done: make(chan struct{}), idle: make(chan struct{}),
		running: map[int64]time.Time{}, reading: map[int64]time.Time{}, log: e.log()}
	if e.Act != nil {
		if act := e.Act(); act != "" {
			g.log = g.log.With("act", act)
		}
	}
	for _, id := range run.Links {
		g.queue = append(g.queue, job{person: id})
	}
	for _, id := range run.Searches {
		g.queue = append(g.queue, job{search: id})
	}
	e.going = g
	e.hire(g)
	go e.run(g)
	g.log.Info("a run started", "run", run.ID, "searches", len(run.Searches), "links", len(run.Links))
}

// hire starts workers until there are SideBySide or one for each thing
// queued. Engine.mu is held.
func (e *Engine) hire(g *going) {
	for g.workers < SideBySide && g.workers < len(g.queue) {
		g.workers++
		go e.work(g)
	}
}

// work does what the run has queued until nothing is left or the run
// is stopped.
func (e *Engine) work(g *going) {
	for {
		e.mu.Lock()
		if g.ctx.Err() != nil || len(g.queue) == 0 {
			g.workers--
			if g.workers == 0 {
				g.ending = true
				close(g.idle)
			}
			e.mu.Unlock()
			return
		}
		j := g.queue[0]
		g.queue = g.queue[1:]
		// A profile taken is being read from now on, so links added
		// meanwhile never queue it a second time, see Add.
		if j.person != 0 {
			g.reading[j.person] = e.Store.Now()
		}
		e.mu.Unlock()
		if j.person != 0 {
			e.read(g.ctx, g, j.person)
		} else {
			e.check(g.ctx, g, j.search)
		}
	}
}

// Added is what came of links Tim added: the people new to the app, those
// here already, how many other links there were, and the run that reads
// the new profiles.
type Added struct {
	Run  int64   `json:"run"`
	New  []int64 `json:"new"`
	Here []int64 `json:"here"`
	// Connected are those here already whom adding marked connected, so
	// Undo can take that back.
	Connected []int64 `json:"connected"`
	Other     int     `json:"other"`
}

// Add adds the people behind the LinkedIn profile links in a text, as
// connected, and has a run read their profiles: the run going, or a new
// one that reads only those. Someone here already is only marked
// connected. Without a key or budget the people are still added, and
// the next run reads them.
func (e *Engine) Add(text string) (Added, error) {
	profiles, other := Links(text)
	out := Added{New: []int64{}, Here: []int64{}, Connected: []int64{}, Other: other}
	if len(profiles) == 0 {
		return out, ErrNoLinks
	}
	for {
		e.mu.Lock()
		g := e.going
		if g == nil || !g.ending {
			break
		}
		// The going run has done all it had and is ending, so nothing
		// joins it. The links start the next.
		e.mu.Unlock()
		<-g.done
	}
	defer e.mu.Unlock()
	if e.closed > 0 {
		return out, ErrClosed
	}
	g := e.going
	var startErr error
	if g == nil {
		if e.Finder == nil {
			startErr = ErrNoKey
		} else if _, err := e.Finder(); errors.Is(err, ErrNoKey) {
			startErr = ErrNoKey
		} else if err != nil {
			startErr = unread{err}
		}
	}
	now := e.Store.Now()
	var run *store.Run
	var read []int64
	err := e.Store.Update(func(d *store.Data) error {
		runID := int64(0)
		switch {
		case g != nil:
			runID = g.id
		case startErr == nil && d.Used(Provider, now) >= d.Int(Provider+"_monthly_searches"):
			startErr = ErrNoBudget
		case startErr == nil:
			runID = d.NewID()
		}
		// Whose profile the going run read already waits for the next,
		// so no profile is read twice in one run.
		var readNow []store.Read
		if g != nil {
			if r := d.Run(g.id); r != nil {
				readNow = r.Reads
			}
		}
		for _, link := range profiles {
			p, isNew, connected := d.AddLink(link, handleOf(link), runID, now)
			switch {
			case isNew:
				out.New = append(out.New, p.ID)
			case connected:
				out.Here, out.Connected = append(out.Here, p.ID), append(out.Connected, p.ID)
			default:
				out.Here = append(out.Here, p.ID)
			}
			done := slices.ContainsFunc(readNow, func(r store.Read) bool { return r.PersonID == p.ID })
			if p.ToRead && !done && !slices.Contains(read, p.ID) {
				read = append(read, p.ID)
			}
		}
		if runID == 0 || len(read) == 0 {
			return nil
		}
		if g != nil {
			if r := d.Run(g.id); r != nil {
				for _, id := range read {
					if !slices.Contains(r.Links, id) {
						r.Links = append(r.Links, id)
					}
				}
			}
		} else {
			run = &store.Run{ID: runID, Kind: "add", StartedAt: now, Searches: []int64{}, Checks: []store.Check{}, Links: read}
			d.Runs = append(d.Runs, run)
		}
		out.Run = runID
		return nil
	})
	if err != nil {
		return out, err
	}
	switch {
	case run != nil:
		e.launch(run)
	case g != nil && out.Run != 0:
		for _, id := range read {
			if _, now := g.reading[id]; !now && !slices.Contains(g.queue, job{person: id}) {
				g.queue = append(g.queue, job{person: id})
			}
		}
		// Profiles before searches, since Tim waits for them.
		slices.SortStableFunc(g.queue, func(a, b job) int { return cmpBool(a.person == 0, b.person == 0) })
		e.hire(g)
		g.log.Info("links joined a run", "run", g.id, "new", len(out.New), "here", len(out.Here))
	}
	return out, startErr
}

// Remove takes out again someone Tim added from a link, for Undo: out
// of the queue of the going run, and out of the app. A read in hand
// finishes and finds nobody to keep it for.
func (e *Engine) Remove(id int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.Store.Update(func(d *store.Data) error { return d.RemoveAdded(id) }); err != nil {
		return err
	}
	if g := e.going; g != nil {
		g.queue = slices.DeleteFunc(g.queue, func(j job) bool { return j.person == id })
	}
	return nil
}

// Paste keeps a profile page Tim copied in his browser as the profile of
// the person id, see search.Pasted and store.Data.Paste, and reports what
// they were before, for Undo, and whether the page gives their name. A
// read of them the going run has not begun is dropped. With as, it puts
// back what they were and reads nothing from the text.
func (e *Engine) Paste(id int64, text string, as *store.Pasted) (was store.Pasted, named bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var runID int64
	if g := e.going; g != nil {
		runID = g.id
	}
	err = e.Store.Update(func(d *store.Data) error {
		p := d.Person(id)
		if p == nil {
			return store.ErrNoPerson
		}
		was = store.Pasted{Text: p.LinkedIn.Text, Name: p.Name, Headline: p.Headline, City: p.City}
		now := e.Store.Now()
		f := store.Pasted{}
		if as != nil {
			f = *as
			f.Text = search.WithoutContact(f.Text)
		} else {
			prof := search.Pasted(text, p.LinkedIn.URL)
			if prof.Text == "" {
				return ErrNothingPasted
			}
			f = store.Pasted{Text: prof.Text, Name: prof.Name, Headline: cmpOr(prof.Headline, p.Headline), City: p.City}
			named = prof.Name != ""
			if city, inNRW := place.PlaceInNRW(prof.Location); inNRW && city != "" {
				f.City = city
			} else if prof.Location != "" && !inNRW {
				city, _, _ = strings.Cut(prof.Location, ",")
				f.City = strings.TrimSpace(city)
			}
		}
		d.Paste(p, f, now)
		score(p, now)
		// The going run no longer waits for them, nor counts them.
		if run := d.Run(runID); run != nil && !p.ToRead && !slices.ContainsFunc(run.Reads, func(r store.Read) bool { return r.PersonID == id }) {
			run.Links = slices.DeleteFunc(run.Links, func(x int64) bool { return x == id })
		}
		return nil
	})
	if err != nil {
		return was, false, err
	}
	if g := e.going; g != nil {
		g.queue = slices.DeleteFunc(g.queue, func(j job) bool { return j.person == id })
	}
	return was, named, nil
}

// ErrNothingPasted is a paste that holds no profile, only white space.
var ErrNothingPasted = errors.New("the paste holds no text")

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

// Reading is whose profiles the going run reads now, with since when, and
// whose wait their turn.
func (e *Engine) Reading() (reading map[int64]time.Time, waiting []int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	reading = map[int64]time.Time{}
	if e.going == nil {
		return reading, nil
	}
	for id, at := range e.going.reading {
		reading[id] = at
	}
	for _, j := range e.going.queue {
		if j.person != 0 {
			waiting = append(waiting, j.person)
		}
	}
	return reading, waiting
}

// run waits until the run's workers are done, and saves its end.
func (e *Engine) run(g *going) {
	defer close(g.done)
	<-g.idle
	ctx := g.ctx

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

// read reads the profile of someone Tim added from a link. Stopped, it
// waits for the next run. A profile Exa's index does not hold yet is not
// read again by itself.
func (e *Engine) read(ctx context.Context, g *going, id int64) {
	defer func() {
		e.mu.Lock()
		delete(g.reading, id)
		e.mu.Unlock()
	}()
	if ctx.Err() != nil {
		return
	}
	start := e.Store.Now()
	var link string
	reserved := false
	err := e.Store.Update(func(d *store.Data) error {
		p := d.Person(id)
		if p == nil || !p.ToRead {
			return nil
		}
		link = p.LinkedIn.URL
		reserved = d.Reserve(Provider, start)
		return nil
	})
	if err != nil || link == "" {
		if err != nil {
			g.log.Error("a profile could not be read", "run", g.id, "person", id, "error", err)
		}
		return
	}
	if !reserved {
		g.log.Info("a profile waits, no Exa searches left this month", "run", g.id, "person", id)
		return
	}
	e.mu.Lock()
	g.reading[id] = start
	e.mu.Unlock()

	var prof search.Profile
	finder, err := e.Finder()
	if err == nil {
		prof, err = ReadProfile(ctx, finder, link, func() (bool, error) {
			var ok bool
			err := e.Store.Update(func(d *store.Data) error {
				ok = d.Reserve(Provider, e.Store.Now())
				return nil
			})
			return ok, err
		})
	}
	if ctx.Err() != nil {
		g.log.Debug("a read was stopped", "run", g.id, "person", id)
		return
	}
	if errors.Is(err, ErrNoBudget) {
		g.log.Info("a profile waits, no Exa searches left this month to ask for it by name", "run", g.id, "person", id)
		return
	}
	r := store.Read{PersonID: id, At: e.Store.Now()}
	r.DurationMS = r.At.Sub(start).Milliseconds()
	err = e.Store.Update(func(d *store.Data) error {
		p := d.Person(id)
		// Tim pasted their profile while Exa was asked, and his wins.
		if p == nil || !p.ToRead {
			return nil
		}
		switch {
		case errors.Is(err, search.ErrNotIndexed):
			// Said without the link, which is personal data.
			r.Error = NotIndexed
			p.ToRead, p.ReadError = false, NotIndexed
		case err != nil:
			// Exa or the network failed. The next run tries again.
			r.Error = err.Error()
			p.ReadError = err.Error()
		default:
			city, inNRW := place.PlaceInNRW(prof.Location)
			if !inNRW {
				city, _, _ = strings.Cut(prof.Location, ",")
			}
			born, reason, roles := careerOf(prof, r.At)
			d.ReadLink(p, store.Found{
				Name: prof.Name, Headline: prof.Headline, City: strings.TrimSpace(city),
				BornAbout: born, AgeReason: reason, Roles: roles, LastActive: prof.LastActive, Text: prof.Text,
			}, r.At)
			score(p, r.At)
		}
		if run := d.Run(g.id); run != nil {
			run.Reads = append(run.Reads, r)
		}
		return nil
	})
	if err != nil {
		g.log.Error("a profile read was not saved", "run", g.id, "person", id, "error", err)
		return
	}
	if r.Error != "" {
		g.log.Warn("a profile could not be read", "run", g.id, "person", id, "ms", r.DurationMS, "error", r.Error)
	} else {
		g.log.Debug("a profile was read", "run", g.id, "person", id, "ms", r.DurationMS)
	}
}

// NotIndexed is why someone added from a link has no profile yet.
const NotIndexed = "Not in Exa's index yet"

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

// Links finds the LinkedIn profile links in a text, like a list pasted
// from a note, each in one spelling and once, and counts the other links
// in it, like a company page. Words that are no link are passed over. A
// link may come without https:// or www.
func Links(text string) (profiles []string, other int) {
	for _, word := range strings.Fields(text) {
		word = strings.Trim(word, "<>()[]\"',;")
		lower := strings.ToLower(word)
		if !strings.Contains(lower, "://") && !strings.HasPrefix(lower, "www.") && !strings.Contains(lower, "linkedin.com") {
			continue
		}
		if !strings.Contains(lower, "://") {
			word = "https://" + word
		}
		if p := linkedIn(word); p != "" {
			if !slices.Contains(profiles, p) {
				profiles = append(profiles, p)
			}
			continue
		}
		other++
	}
	return profiles, other
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
	if !strings.HasPrefix(path, "/in/") || strings.Count(path, "/") != 2 || len(path) == len("/in/") {
		return ""
	}
	return "https://www.linkedin.com" + path
}

// handleOf is the last part of a profile's link.
func handleOf(profile string) string {
	return profile[strings.LastIndex(profile, "/")+1:]
}

// cmpBool puts false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
