package api

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
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
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	KnownAs        string         `json:"known_as"`
	Headline       string         `json:"headline"`
	City           string         `json:"city"`
	Fit            string         `json:"fit"`
	FirstSeen      time.Time      `json:"first_seen"`
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
}

func listPerson(p *store.Person) personJSON {
	out := personJSON{
		ID: p.ID, Name: p.Name, Headline: p.Headline, City: p.City, Fit: "other", FirstSeen: p.FirstSeen,
		FitScore: p.Score, Connected: p.Connected, Signals: p.Signals, Profiles: []profileJSON{},
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

// readsNow asks the engine whose profiles it reads, and measures how
// long a read takes from the last ones.
func (a *API) readsNow(d *store.Data) reads {
	reading, waiting := a.Engine.Reading()
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

func detailPerson(d *store.Data, p *store.Person) personDetail {
	out := personDetail{personJSON: listPerson(p), Appearances: []any{}, Affiliations: []any{},
		Notes: p.Notes, FitEvidence: p.Evidence, Sightings: []sightingJSON{},
		Draft: p.Draft, DraftedAt: ptrTime(p.DraftedAt)}
	if p.LinkedIn.Text != "" {
		out.ProfileReadAt = ptrTime(p.LinkedIn.TextAt)
	}
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
	a.Store.Read(func(d *store.Data) {
		var people []*store.Person
		words := q.Get("q")
		if len(profiles) > 0 || other > 0 {
			words = ""
		}
		people, out.Counts, err = d.ListPeople(store.PeopleQuery{Q: words, Sort: q.Get("sort"), Filter: q.Get("filter"), Now: a.Store.Now()})
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
		}
		rs := a.readsNow(d)
		out.People = make([]personJSON, 0, len(people))
		for _, p := range people {
			pj := listPerson(p)
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
		Wait string `json:"wait,omitempty"`
	}{Added: added}
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

func (a *API) sendPerson(w http.ResponseWriter, id int64) {
	var out *personDetail
	a.Store.Read(func(d *store.Data) {
		if p := d.Person(id); p != nil {
			v := detailPerson(d, p)
			a.readsNow(d).mark(p, &v.personJSON)
			out = &v
		}
	})
	if out == nil {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	send(w, http.StatusOK, out)
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
