package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/store"
)

type nowJSON struct {
	Kind  string    `json:"kind"`
	Label string    `json:"label"`
	Since time.Time `json:"since"`
}

// progressJSON says how far a going run is. A run only asks searches, so
// it has no reads and no lookups.
type progressJSON struct {
	Checks          int       `json:"checks"`
	ChecksDone      int       `json:"checks_done"`
	Reads           int       `json:"reads"`
	ReadsDone       int       `json:"reads_done"`
	ReadsExpected   int       `json:"reads_expected"`
	Lookups         int       `json:"lookups"`
	LookupsDone     int       `json:"lookups_done"`
	LookupsExpected int       `json:"lookups_expected"`
	Now             []nowJSON `json:"now"`
	SecondsLeft     *int64    `json:"seconds_left"`
}

type runJSON struct {
	ID               int64         `json:"id"`
	Kind             string        `json:"kind"`
	State            string        `json:"state"`
	PagesRead        int           `json:"pages_read"`
	ReadsFailed      int           `json:"reads_failed"`
	StartupsLookedUp int           `json:"startups_looked_up"`
	LookupsFailed    int           `json:"lookups_failed"`
	FitsNew          int           `json:"fits_new"`
	Progress         *progressJSON `json:"progress"`
	StartedAt        time.Time     `json:"started_at"`
	FinishedAt       *time.Time    `json:"finished_at"`
	SourcesChecked   int           `json:"sources_checked"`
	EventsFound      int           `json:"events_found"`
	EventsNew        int           `json:"events_new"`
	PeopleNew        int           `json:"people_new"`
	Errors           int           `json:"errors"`
}

// recentChecks is how many of the last checks say how long one takes.
const recentChecks = 200

func runOf(d *store.Data, r *store.Run, going int64, now map[int64]time.Time) runJSON {
	out := runJSON{ID: r.ID, Kind: r.Kind, StartedAt: r.StartedAt, FinishedAt: ptrTime(r.FinishedAt), State: "finished"}
	switch {
	case r.ID == going:
		out.State, out.FinishedAt = "going", nil
	case r.Ended == "stopped":
		out.State = "stopped"
	case r.Ended == "interrupted":
		out.State = "interrupted"
	}
	seen := map[int64]bool{}
	for _, c := range r.Checks {
		seen[c.SearchID] = true
		out.PeopleNew += c.PeopleNew
		if c.Error != "" {
			out.Errors++
		}
	}
	out.SourcesChecked = len(seen)
	for _, p := range d.People {
		if p.FirstRunID == r.ID && p.Score > 0 {
			out.FitsNew++
		}
	}
	if out.State == "going" {
		pr := &progressJSON{Checks: len(r.Searches), ChecksDone: len(r.Checks), Now: []nowJSON{}}
		for id, since := range now {
			label := ""
			if s := d.Search(id); s != nil {
				label = s.Query
			}
			pr.Now = append(pr.Now, nowJSON{Kind: "check", Label: label, Since: since})
		}
		slices.SortFunc(pr.Now, func(a, b nowJSON) int { return a.Since.Compare(b.Since) })
		// How long is left, from how long the last checks took, side by
		// side. Nothing is said until there is something to measure.
		var total, n int64
		for i := len(d.Runs) - 1; i >= 0 && n < recentChecks; i-- {
			for j := len(d.Runs[i].Checks) - 1; j >= 0 && n < recentChecks; j-- {
				total += d.Runs[i].Checks[j].DurationMS
				n++
			}
		}
		if n > 0 {
			left := int64(pr.Checks-pr.ChecksDone) * (total / n) / engine.SideBySide / 1000
			pr.SecondsLeft = &left
		}
		out.Progress = pr
	}
	return out
}

func (a *API) runs(w http.ResponseWriter, r *http.Request) {
	going, now := a.Engine.Going()
	var out struct {
		Runs []runJSON `json:"runs"`
	}
	a.Store.Read(func(d *store.Data) {
		out.Runs = make([]runJSON, 0, len(d.Runs))
		for i := len(d.Runs) - 1; i >= 0 && len(out.Runs) < 50; i-- {
			out.Runs = append(out.Runs, runOf(d, d.Runs[i], going, now))
		}
	})
	send(w, http.StatusOK, out)
}

func (a *API) currentRun(w http.ResponseWriter, r *http.Request) {
	going, now := a.Engine.Going()
	var out struct {
		Run *runJSON `json:"run"`
	}
	a.Store.Read(func(d *store.Data) {
		if run := d.Run(going); run != nil {
			v := runOf(d, run, going, now)
			out.Run = &v
		}
	})
	send(w, http.StatusOK, out)
}

func (a *API) startRun(w http.ResponseWriter, r *http.Request) {
	id, started, err := a.Engine.Start("manual", 0)
	if err != nil {
		a.startFailed(w, r, err)
		return
	}
	send(w, http.StatusOK, map[string]any{"run_id": id, "started": started})
}

func (a *API) stopRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !a.Engine.Stop(id) {
		fail(w, http.StatusConflict, "That run is not going")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type checkJSON struct {
	ID          int    `json:"id"`
	Source      any    `json:"source"`
	Mode        string `json:"mode"`
	HTTPStatus  int    `json:"http_status"`
	EventsFound int    `json:"events_found"`
	EventsKept  int    `json:"events_kept"`
	PeopleFound int    `json:"people_found"`
	Error       string `json:"error"`
	DurationMS  int64  `json:"duration_ms"`
	CheckedAt   string `json:"checked_at"`
	FetchID     *int64 `json:"fetch_id"`
}

type failureJSON struct {
	What   string `json:"what"`
	Source any    `json:"source"`
	Reason string `json:"reason"`
	Action string `json:"action"`
	Count  int    `json:"count"`
	Detail string `json:"detail"`
}

func (a *API) run(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	going, now := a.Engine.Going()
	var out *struct {
		Run      runJSON       `json:"run"`
		Checks   []checkJSON   `json:"checks"`
		Failures []failureJSON `json:"failures"`
	}
	a.Store.Read(func(d *store.Data) {
		run := d.Run(id)
		if run == nil {
			return
		}
		out = &struct {
			Run      runJSON       `json:"run"`
			Checks   []checkJSON   `json:"checks"`
			Failures []failureJSON `json:"failures"`
		}{Run: runOf(d, run, going, now), Checks: []checkJSON{}, Failures: []failureJSON{}}
		for i, c := range run.Checks {
			ref := map[string]any{"id": c.SearchID, "name": "a search since removed", "url": "", "status": "retired"}
			if s := d.Search(c.SearchID); s != nil {
				ref["name"], ref["status"] = s.Query, s.Status
			}
			status := 200
			if c.Error != "" {
				status = 0
				out.Failures = append(out.Failures, failureJSON{What: "check", Source: ref, Reason: "Exa did not answer this search",
					Action: "check", Count: 1, Detail: c.Error})
			}
			out.Checks = append(out.Checks, checkJSON{ID: i + 1, Source: ref, Mode: "search", HTTPStatus: status,
				PeopleFound: c.PeopleFound, Error: c.Error, DurationMS: c.DurationMS, CheckedAt: c.At.Format(time.RFC3339)})
		}
	})
	if out == nil {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	send(w, http.StatusOK, out)
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Settings []store.Setting `json:"settings"`
	}
	a.Store.Read(func(d *store.Data) { out.Settings = d.AllSettings() })
	send(w, http.StatusOK, out)
}

func (a *API) patchSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var body struct {
		Value json.RawMessage `json:"value"`
	}
	if !decode(w, r, &body) {
		return
	}
	var out store.Setting
	if !a.update(w, r, func(d *store.Data) error {
		if _, ok := d.Setting(key); !ok {
			return errNotFound
		}
		if err := d.SetSetting(key, body.Value, a.Store.Now()); err != nil {
			return errBad{err}
		}
		out, _ = d.Setting(key)
		return nil
	}) {
		return
	}
	send(w, http.StatusOK, out)
}

// keys says which search keys are saved, never the keys themselves.
func (a *API) keys(w http.ResponseWriter, r *http.Request) {
	var used, budget int
	a.Store.Read(func(d *store.Data) {
		now := a.Store.Now()
		used, budget = d.Used(engine.Provider, now), d.Int(engine.Provider+"_monthly_searches")
	})
	send(w, http.StatusOK, map[string]any{"keys": []store.Use{{Provider: engine.Provider, Used: used, Budget: budget, Set: a.Keys != nil && a.Keys.Has()}}})
}

func (a *API) setKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &body) {
		return
	}
	if a.Keys == nil {
		fail(w, http.StatusNotImplemented, "Keys cannot be saved here")
		return
	}
	if err := a.Keys.Set(body.Key); err != nil {
		fail(w, http.StatusBadRequest, capital(err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
