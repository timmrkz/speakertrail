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

// listExclusions lists the exclusions in the order of their phrases. Who
// two of them match counts for the first.
func listExclusions(d *store.Data) []exclusionJSON {
	per := map[string]int{}
	for _, phrase := range d.Hidden() {
		per[phrase]++
	}
	out := []exclusionJSON{}
	for _, x := range d.SortedExclusions() {
		out = append(out, exclusionJSON{ID: x.ID, Phrase: x.Phrase, People: per[x.Phrase]})
	}
	return out
}

func (a *API) exclusions(w http.ResponseWriter, r *http.Request) {
	var out []exclusionJSON
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
	var id int64
	err := a.Store.Update(func(d *store.Data) error {
		x, err := d.AddExclusion(body.Phrase, a.Store.Now())
		if err != nil {
			return err
		}
		id = x.ID
		return nil
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
	var out *exclusionJSON
	a.Store.Read(func(d *store.Data) {
		for _, x := range listExclusions(d) {
			if x.ID == id {
				out = &x
			}
		}
	})
	if out == nil {
		a.internal(w, r, errors.New("the new exclusion is gone"))
		return
	}
	send(w, http.StatusCreated, out)
}

// deleteExclusion removes one. The people it hid come back.
func (a *API) deleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if a.update(w, r, func(d *store.Data) error {
		if !d.DeleteExclusion(id) {
			return errNotFound
		}
		return nil
	}) {
		w.WriteHeader(http.StatusNoContent)
	}
}
