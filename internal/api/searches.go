package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/place"
	"github.com/timmrkz/speakertrail/internal/store"
)

type lastCheckJSON struct {
	EventsFound   int    `json:"events_found"`
	StartupsFound int    `json:"startups_found"`
	PeopleFound   int    `json:"people_found"`
	HTTPStatus    int    `json:"http_status"`
	Mode          string `json:"mode"`
	Error         string `json:"error"`
}

// searchJSON is a profile search in the shape of a source, which is what
// the interface knows it as.
type searchJSON struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`
	URL            *string        `json:"url"`
	Query          string         `json:"query"`
	Category       string         `json:"category"`
	City           string         `json:"city"`
	Status         string         `json:"status"`
	FetchMode      string         `json:"fetch_mode"`
	Notes          string         `json:"notes"`
	LastCheckedAt  *time.Time     `json:"last_checked_at"`
	NextCheckAt    *time.Time     `json:"next_check_at"`
	Checks         int            `json:"checks"`
	EmptyInRow     int            `json:"empty_checks_in_row"`
	Points         float64        `json:"points"`
	LastCheck      *lastCheckJSON `json:"last_check"`
	Health         string         `json:"health"`
	HealthNote     string         `json:"health_note"`
	DiscoveredFrom string         `json:"discovered_from"`
	People         int            `json:"people"`
	PeopleKept     int            `json:"people_kept"`
}

func searchOf(d *store.Data, s *store.Search) searchJSON {
	out := searchJSON{
		ID: s.ID, Name: s.Query, Kind: "profile_search", Query: s.Query, Category: "Profile search",
		City: place.CityOf(s.Query), Status: s.Status, FetchMode: "auto", Notes: s.Notes,
		LastCheckedAt: ptrTime(s.LastRun), NextCheckAt: ptrTime(s.NextRun), Checks: s.Runs,
		EmptyInRow: s.EmptyInRow, Points: s.Points, Health: "never",
	}
	if c := s.Last; c != nil {
		out.LastCheck = &lastCheckJSON{PeopleFound: c.PeopleFound, Mode: "search", Error: c.Error, HTTPStatus: 200}
		out.Health = "ok"
		if c.Error != "" {
			out.LastCheck.HTTPStatus = 0
			out.Health, out.HealthNote = "error", "The last run of this search failed: "+c.Error
		}
	}
	for _, p := range d.People {
		for _, f := range p.FoundBy {
			if f.SearchID == s.ID {
				out.People++
				if p.Decision == "kept" {
					out.PeopleKept++
				}
			}
		}
	}
	return out
}

func (a *API) searches(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var out struct {
		Sources []searchJSON `json:"sources"`
	}
	a.Store.Read(func(d *store.Data) {
		out.Sources = make([]searchJSON, 0, len(d.Searches))
		for _, s := range d.Searches {
			if q == "" || strings.Contains(strings.ToLower(s.Query), q) {
				out.Sources = append(out.Sources, searchOf(d, s))
			}
		}
	})
	// The newest first, as a search just added shows at the top.
	slices.SortStableFunc(out.Sources, func(a, b searchJSON) int {
		return int(b.ID - a.ID)
	})
	send(w, http.StatusOK, out)
}

func (a *API) sendSearch(w http.ResponseWriter, status int, id int64) {
	var out *searchJSON
	a.Store.Read(func(d *store.Data) {
		if s := d.Search(id); s != nil {
			v := searchOf(d, s)
			out = &v
		}
	})
	if out == nil {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	send(w, status, out)
}

func (a *API) addSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
		Kind  string `json:"kind"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Kind != "" && body.Kind != "profile_search" {
		fail(w, http.StatusBadRequest, "Only profile searches can be added")
		return
	}
	var id int64
	err := a.Store.Update(func(d *store.Data) error {
		s, err := d.AddSearch(body.Query, 0, a.Store.Now())
		if err != nil {
			return err
		}
		id = s.ID
		return nil
	})
	switch {
	case errors.Is(err, store.ErrSearchExists):
		fail(w, http.StatusConflict, "That search is there already")
	case err != nil:
		fail(w, http.StatusBadRequest, capital(err.Error()))
	default:
		a.sendSearch(w, http.StatusCreated, id)
	}
}

// patchSearch pauses a search, status retired, or resumes it, any other
// status, and changes its notes.
func (a *API) patchSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status *string `json:"status"`
		Notes  *string `json:"notes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !a.update(w, r, func(d *store.Data) error {
		s := d.Search(id)
		if s == nil {
			return errNotFound
		}
		now := a.Store.Now()
		if body.Status != nil {
			if *body.Status == "retired" {
				s.Pause(now)
			} else {
				s.Resume(now)
			}
		}
		if body.Notes != nil {
			s.Notes = *body.Notes
		}
		return nil
	}) {
		return
	}
	a.sendSearch(w, http.StatusOK, id)
}

// checkSearch runs one search now, as a run of its own.
func (a *API) checkSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	found := false
	a.Store.Read(func(d *store.Data) { found = d.Search(id) != nil })
	if !found {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	if _, started, err := a.Engine.Start("check", id); err != nil {
		a.startFailed(w, r, err)
		return
	} else if !started {
		fail(w, http.StatusConflict, "A run is going. Check again once it has ended")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) startFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, engine.ErrNoKey), errors.Is(err, engine.ErrNoBudget), errors.Is(err, engine.ErrNothing):
		fail(w, http.StatusConflict, capital(err.Error()))
	default:
		a.internal(w, r, err)
	}
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
