package api

import (
	"errors"
	"net/http"

	"github.com/timmrkz/speakertrail/internal/store"
)

// exclusionJSON is one of Tim's words or phrases that leave a person out,
// with how many people found so far it hides.
type exclusionJSON struct {
	ID     int64  `json:"id"`
	Phrase string `json:"phrase"`
	People int    `json:"people"`
}

// exclusionsJSON is every exclusion, in the order of their phrases, and
// how many people they hide together. Adding and removing one answers it
// too, since either moves people between the others, so the window sorts
// and counts nothing itself, issue #105.
type exclusionsJSON struct {
	Exclusions []exclusionJSON `json:"exclusions"`
	Hiding     int             `json:"hiding"`
}

// listExclusions lists the exclusions in the order of their phrases. Who
// two of them match counts for the first.
func listExclusions(d *store.Data) exclusionsJSON {
	hidden := d.Hidden()
	per := map[string]int{}
	for _, phrase := range hidden {
		per[phrase]++
	}
	out := exclusionsJSON{Exclusions: []exclusionJSON{}, Hiding: len(hidden)}
	for _, x := range d.SortedExclusions() {
		out.Exclusions = append(out.Exclusions, exclusionJSON{ID: x.ID, Phrase: x.Phrase, People: per[x.Phrase]})
	}
	return out
}

func (a *API) exclusions(w http.ResponseWriter, r *http.Request) {
	var out exclusionsJSON
	a.Store.Read(func(d *store.Data) { out = listExclusions(d) })
	send(w, http.StatusOK, out)
}

func (a *API) addExclusion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phrase string `json:"phrase"`
	}
	if !decode(w, r, &body) {
		return
	}
	err := a.Store.Add(func(d *store.Data) (store.Thing, error) {
		x, err := d.AddExclusion(body.Phrase, a.Store.Now())
		if err != nil {
			return store.Thing{}, err
		}
		return store.ExclusionThing(x.ID), nil
	})
	switch {
	case errors.Is(err, store.ErrExclusionExists):
		fail(w, http.StatusConflict, "This exclusion is already there")
		return
	case errors.Is(err, store.ErrNoPhrase), errors.Is(err, store.ErrLongPhrase):
		fail(w, http.StatusBadRequest, capital(err.Error()))
		return
	case err != nil:
		a.internal(w, r, err)
		return
	}
	var out exclusionsJSON
	a.Store.Read(func(d *store.Data) { out = listExclusions(d) })
	send(w, http.StatusCreated, out)
}

// deleteExclusion removes one, and answers every exclusion left. The
// people it hid come back. Undo in the Edit menu puts it back.
func (a *API) deleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if a.edit(w, r, store.ExclusionThing(id), func(d *store.Data) error {
		if !d.DeleteExclusion(id) {
			return errNotFound
		}
		return nil
	}) {
		a.exclusions(w, r)
	}
}
