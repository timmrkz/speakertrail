package api

import (
	"net/http"
	"slices"
	"time"

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

func (a *API) people(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out struct {
		People []personJSON   `json:"people"`
		Counts map[string]int `json:"counts"`
	}
	var err error
	a.Store.Read(func(d *store.Data) {
		var people []*store.Person
		now := a.Store.Now()
		people, out.Counts, err = d.ListPeople(store.PeopleQuery{Q: q.Get("q"), Sort: q.Get("sort"), Filter: q.Get("filter"), Now: now})
		out.People = make([]personJSON, 0, len(people))
		fitOf := d.Fitter(now)
		for _, p := range people {
			out.People = append(out.People, listPerson(p, fitOf(p)))
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
			v := detailPerson(d, p, a.Store.Now())
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
