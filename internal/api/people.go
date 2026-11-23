package api

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/rubric"
	"github.com/timmrkz/speakertrail/internal/store"
)

type profileJSON struct {
	ID       int64  `json:"id"`
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Review   string `json:"review"`
}

type linkedInJSON struct {
	// URL is the profile the LinkedIn button shows and opens.
	URL        string     `json:"url"`
	BornAbout  *int       `json:"born_about"`
	AgeReason  string     `json:"age_reason"`
	Roles      string     `json:"roles"`
	LastActive *time.Time `json:"last_active"`
}

// personJSON is a person in the list.
type personJSON struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	KnownAs   string    `json:"known_as"`
	Headline  string    `json:"headline"`
	City      string    `json:"city"`
	Fit       string    `json:"fit"`
	FirstSeen time.Time `json:"first_seen"`
	// FitScore is the fit from 0 to 100, see store.Data.Fit.
	FitScore       int            `json:"fit_score"`
	Connected      bool           `json:"connected"`
	Signals        []store.Signal `json:"signals"`
	Appearances    int            `json:"appearances"`
	NextAppearance any            `json:"next_appearance"`
	Profiles       []profileJSON  `json:"profiles"`
	Activity       any            `json:"activity"`
	LinkedIn       *linkedInJSON  `json:"linkedin"`
	// Read says where the profile of someone added from a link stands:
	// reading now, waiting its turn in the run, next for a run that
	// reads it later, not_indexed when Exa's index does not hold it yet,
	// failed when the last read failed, or empty once read. ReadSince is
	// when the read began, and ReadMS about how long one takes, measured
	// from the last reads, so the row can fill as it goes.
	Read      string     `json:"read"`
	ReadSince *time.Time `json:"read_since"`
	ReadMS    *int64     `json:"read_ms"`
	ReadError string     `json:"read_error,omitempty"`
}

type sightingJSON struct {
	SourceID  int64     `json:"source_id"`
	Source    string    `json:"source"`
	CheckedAt time.Time `json:"checked_at"`
}

// personDetail is a person on their sheet.
type personDetail struct {
	personJSON
	Appearances  []any          `json:"appearances"`
	Notes        string         `json:"notes"`
	FitEvidence  string         `json:"fit_evidence"`
	Affiliations []any          `json:"affiliations"`
	Sightings    []sightingJSON `json:"sightings"`
	// Draft is the first message to them, as the language model wrote it
	// at DraftedAt and as Tim changed it since, empty before the first.
	Draft     string     `json:"draft"`
	DraftedAt *time.Time `json:"drafted_at"`
	// ProfileReadAt is when Exa's index gave the profile text a draft is
	// written from, nil while the app has none and a draft reads it first.
	ProfileReadAt *time.Time `json:"profile_read_at"`
	// FitParts are what the fit adds up, each with its points and why.
	FitParts []fitPartJSON `json:"fit_parts"`
}

type fitPartJSON struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Points int    `json:"points"`
	Why    string `json:"why"`
	// Lines are what the part shows under it, each with a tick or a
	// cross: the signals of Who they are with their passages, or for the
	// other parts their why.
	Lines []fitLineJSON `json:"lines"`
}

// fitLineJSON is one line under a part of the fit. Mark is "for",
// "against" or "neither".
type fitLineJSON struct {
	Mark    string `json:"mark"`
	Text    string `json:"text"`
	Passage string `json:"passage"`
	FoundIn string `json:"found_in"`
}

// fitParts gives the parts of a fit with the lines under each, so the
// person shows one list: every signal under the part it belongs to, see
// rubric.PartOf. A signal under Active lately or Younger says what the
// part's why says, so only Who they are shows its signals.
func fitParts(parts []rubric.Part, signals []store.Signal) []fitPartJSON {
	marks := map[int]string{1: "for", 0: "neither", -1: "against"}
	out := make([]fitPartJSON, 0, len(parts))
	for _, pt := range parts {
		f := fitPartJSON{Key: pt.Key, Label: pt.Label, Points: pt.Points, Why: pt.Why, Lines: []fitLineJSON{}}
		if pt.Key == "who" {
			for _, s := range signals {
				if rubric.PartOf(s.Key) == "who" {
					m := "against"
					if s.For {
						m = "for"
					}
					f.Lines = append(f.Lines, fitLineJSON{Mark: m, Text: s.Label, Passage: s.Passage, FoundIn: s.FoundIn})
				}
			}
		}
		if len(f.Lines) == 0 {
			f.Lines = append(f.Lines, fitLineJSON{Mark: marks[pt.Mark], Text: pt.Why})
		}
		out = append(out, f)
	}
	return out
}

// listPerson gives a person as the list shows them, with their fit.
func listPerson(p *store.Person, fit int) personJSON {
	out := personJSON{
		ID: p.ID, Name: p.Name, Headline: p.Headline, City: p.City, Fit: "other", FirstSeen: p.FirstSeen,
		FitScore: fit, Connected: p.Connected, Signals: p.Signals, Profiles: []profileJSON{},
	}
	if out.Signals == nil {
		out.Signals = []store.Signal{}
	}
	if l := p.LinkedIn; l.URL != "" {
		// A person has one profile, so it goes by the person's id.
		out.Profiles = append(out.Profiles, profileJSON{ID: p.ID, Platform: "linkedin", URL: l.URL, Review: l.Review})
		if !p.Rejected() {
			lj := &linkedInJSON{URL: l.URL, AgeReason: l.AgeReason, Roles: l.Roles, LastActive: ptrTime(l.LastActive)}
			if l.BornAbout != 0 {
				born := l.BornAbout
				lj.BornAbout = &born
			}
			out.LinkedIn = lj
		}
	}
	return out
}

// reads is where the reads of the going run stand, see personJSON.Read.
type reads struct {
	reading map[int64]time.Time
	waiting []int64
	ms      *int64
}

// readsNow is whose profiles the engine reads and whose wait, as it said
// before the store was read, with how long a read takes, measured from
// the last ones. The engine is asked first and never inside Store.Read,
// since Engine.Add holds its own lock while it updates the store.
func readsNow(d *store.Data, reading map[int64]time.Time, waiting []int64) reads {
	out := reads{reading: reading, waiting: waiting}
	var total, n int64
	for i := len(d.Runs) - 1; i >= 0 && n < recentChecks; i-- {
		for j := len(d.Runs[i].Reads) - 1; j >= 0 && n < recentChecks; j-- {
			total += d.Runs[i].Reads[j].DurationMS
			n++
		}
	}
	if n > 0 {
		ms := total / n
		out.ms = &ms
	}
	return out
}

// mark says where a person's profile read stands.
func (r reads) mark(p *store.Person, out *personJSON) {
	out.ReadError = p.ReadError
	if since, ok := r.reading[p.ID]; ok {
		out.Read, out.ReadSince, out.ReadMS = "reading", ptrTime(since), r.ms
		return
	}
	switch {
	case slices.Contains(r.waiting, p.ID):
		out.Read = "waiting"
	case p.ToRead:
		out.Read = "next"
	case p.ReadError == engine.NotIndexed:
		out.Read = "not_indexed"
	case p.ReadError != "":
		out.Read = "failed"
	}
}

func detailPerson(d *store.Data, p *store.Person, now time.Time) personDetail {
	fit, parts := d.Fit(p, now)
	out := personDetail{personJSON: listPerson(p, fit), Appearances: []any{}, Affiliations: []any{},
		Notes: p.Notes, FitEvidence: p.Evidence, Sightings: []sightingJSON{},
		Draft: p.Draft, DraftedAt: ptrTime(p.DraftedAt)}
	if p.LinkedIn.Text != "" {
		out.ProfileReadAt = ptrTime(p.LinkedIn.TextAt)
	}
	out.FitParts = fitParts(parts, p.Signals)
	for _, s := range p.FoundBy {
		name := "a search since removed"
		if search := d.Search(s.SearchID); search != nil {
			name = search.Query
		}
		out.Sightings = append(out.Sightings, sightingJSON{SourceID: s.SearchID, Source: name, CheckedAt: s.At})
	}
	slices.SortFunc(out.Sightings, func(a, b sightingJSON) int { return b.CheckedAt.Compare(a.CheckedAt) })
	return out
}

// linksJSON is what a search for links holds: how many profiles are new
// to the app, who is here already, by id, and how many other links,
// like a company page.
type linksJSON struct {
	New   int     `json:"new"`
	Here  []int64 `json:"here"`
	Other int     `json:"other"`
	// Act is what Run turns into: add for profiles new to the app, open
	// for the one person a link leads to who is here, none for links that
	// add nobody, see actOf.
	Act string `json:"act"`
}

// actOf is what adding links does, the same for the search that holds
// them and for Add.
func actOf(isNew int, here []int64, other int) string {
	switch {
	case isNew > 0:
		return "add"
	case len(here) == 1 && other == 0:
		return "open"
	}
	return "none"
}

func (a *API) people(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out struct {
		People []personJSON   `json:"people"`
		Counts map[string]int `json:"counts"`
		// Links is set when the search holds links, see linksJSON. The
		// people are then whom the links lead to, under any filter.
		Links *linksJSON `json:"links"`
	}
	var err error
	profiles, other := engine.Links(q.Get("q"))
	reading, waiting := a.Engine.Reading()
	a.Store.Read(func(d *store.Data) {
		var people []*store.Person
		now := a.Store.Now()
		words := q.Get("q")
		if len(profiles) > 0 || other > 0 {
			words = ""
		}
		people, out.Counts, err = d.ListPeople(store.PeopleQuery{Q: words, Sort: q.Get("sort"), Filter: q.Get("filter"), Now: now})
		if len(profiles) > 0 || other > 0 {
			out.Links = &linksJSON{Here: []int64{}, Other: other}
			people = people[:0]
			for _, link := range profiles {
				if p := d.PersonByProfile(link); p != nil {
					out.Links.Here = append(out.Links.Here, p.ID)
					people = append(people, p)
				} else {
					out.Links.New++
				}
			}
			out.Links.Act = actOf(out.Links.New, out.Links.Here, other)
		}
		rs := readsNow(d, reading, waiting)
		out.People = make([]personJSON, 0, len(people))
		fitOf := d.Fitter(now)
		for _, p := range people {
			pj := listPerson(p, fitOf(p))
			rs.mark(p, &pj)
			out.People = append(out.People, pj)
		}
	})
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, f := range store.Filters {
		out.Counts[f] += 0
	}
	send(w, http.StatusOK, out)
}

// addPeople adds the people behind the LinkedIn links in a text, as
// connected, and has a run read their profiles. Without a key or budget
// they are added all the same, and wait says why their profiles wait
// for the next run.
func (a *API) addPeople(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	added, err := a.Engine.Add(in.Text)
	out := struct {
		engine.Added
		// Open is the one person the links lead to who was here
		// already, for People to open, when they add nobody new.
		Open int64  `json:"open,omitempty"`
		Wait string `json:"wait,omitempty"`
	}{Added: added}
	if actOf(len(added.New), added.Here, added.Other) == "open" {
		out.Open = added.Here[0]
	}
	switch {
	case errors.Is(err, engine.ErrNoLinks):
		fail(w, http.StatusBadRequest, capital(err.Error()))
		return
	case errors.Is(err, engine.ErrClosed):
		fail(w, http.StatusConflict, capital(err.Error()))
		return
	case errors.Is(err, engine.ErrNoKey), errors.Is(err, engine.ErrUnread), errors.Is(err, engine.ErrNoBudget):
		out.Wait = capital(err.Error())
	case err != nil:
		a.internal(w, r, err)
		return
	}
	send(w, http.StatusOK, out)
}

// removePerson takes out someone added from a link, for Undo.
func (a *API) removePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := a.Engine.Remove(id); {
	case errors.Is(err, store.ErrNoPerson):
		fail(w, http.StatusNotFound, "Not found")
	case errors.Is(err, store.ErrNotAdded):
		fail(w, http.StatusConflict, capital(err.Error()))
	case err != nil:
		a.internal(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) sendPerson(w http.ResponseWriter, id int64) {
	if out := a.detail(id); out != nil {
		send(w, http.StatusOK, out)
		return
	}
	fail(w, http.StatusNotFound, "Not found")
}

// detail is everything the app knows about one person, nil for nobody.
func (a *API) detail(id int64) *personDetail {
	var out *personDetail
	reading, waiting := a.Engine.Reading()
	a.Store.Read(func(d *store.Data) {
		if p := d.Person(id); p != nil {
			v := detailPerson(d, p, a.Store.Now())
			readsNow(d, reading, waiting).mark(p, &v.personJSON)
			out = &v
		}
	})
	return out
}

// pasteProfile keeps a LinkedIn page Tim copied in his browser as a
// person's profile, see engine.Paste. With as, it puts back what Undo
// had kept.
func (a *API) pasteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string        `json:"text"`
		As   *store.Pasted `json:"as"`
	}
	if !decode(w, r, &body) {
		return
	}
	was, named, err := a.Engine.Paste(id, body.Text, body.As)
	switch {
	case errors.Is(err, store.ErrNoPerson):
		fail(w, http.StatusNotFound, "Not found")
		return
	case errors.Is(err, engine.ErrNothingPasted):
		fail(w, http.StatusBadRequest, capital(err.Error()))
		return
	case err != nil:
		a.internal(w, r, err)
		return
	}
	out := a.detail(id)
	if out == nil {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	send(w, http.StatusOK, struct {
		Person *personDetail `json:"person"`
		Was    store.Pasted  `json:"was"`
		Named  bool          `json:"named"`
	}{out, was, named})
}

func (a *API) person(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		a.sendPerson(w, id)
	}
}

func (a *API) patchPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Notes *string `json:"notes"`
		// Connected is whether Tim connected with the person on LinkedIn.
		Connected *bool `json:"connected"`
		// Draft is the first message as Tim changed it.
		Draft *string `json:"draft"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !a.update(w, r, func(d *store.Data) error {
		p := d.Person(id)
		if p == nil {
			return errNotFound
		}
		now := a.Store.Now()
		if body.Connected != nil {
			p.Connect(*body.Connected, now)
		}
		if body.Notes != nil {
			p.Notes, p.UpdatedAt = *body.Notes, now
		}
		if body.Draft != nil {
			p.Draft, p.UpdatedAt = *body.Draft, now
			// An emptied message was drafted at no time.
			if p.Draft == "" {
				p.DraftedAt = time.Time{}
			}
		}
		return nil
	}) {
		return
	}
	a.sendPerson(w, id)
}

// patchProfile says whether a LinkedIn profile is the person's own. A
// profile search finds people by their profile, so it is confirmed unless
// Tim says otherwise.
func (a *API) patchProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Review string `json:"review"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Review != "confirmed" && body.Review != "rejected" && body.Review != "open" {
		fail(w, http.StatusBadRequest, "review must be open, confirmed or rejected")
		return
	}
	if a.update(w, r, func(d *store.Data) error {
		p := d.Person(id)
		if p == nil || p.LinkedIn.URL == "" {
			return errNotFound
		}
		p.LinkedIn.Review, p.UpdatedAt = body.Review, a.Store.Now()
		return nil
	}) {
		w.WriteHeader(http.StatusNoContent)
	}
}
