package api

import (
	"net/http"
	"slices"
	"time"

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
		if l.Review != "rejected" {
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

func detailPerson(d *store.Data, p *store.Person) personDetail {
	out := personDetail{personJSON: listPerson(p), Appearances: []any{}, Affiliations: []any{},
		Notes: p.Notes, FitEvidence: p.Evidence, Sightings: []sightingJSON{}}
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

func (a *API) people(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out struct {
		People []personJSON   `json:"people"`
		Counts map[string]int `json:"counts"`
	}
	var err error
	a.Store.Read(func(d *store.Data) {
		var people []*store.Person
		people, out.Counts, err = d.ListPeople(store.PeopleQuery{Q: q.Get("q"), Sort: q.Get("sort"), Filter: q.Get("filter"), Now: a.Store.Now()})
		out.People = make([]personJSON, 0, len(people))
		for _, p := range people {
			out.People = append(out.People, listPerson(p))
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

func (a *API) sendPerson(w http.ResponseWriter, id int64) {
	var out *personDetail
	a.Store.Read(func(d *store.Data) {
		if p := d.Person(id); p != nil {
			v := detailPerson(d, p)
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
