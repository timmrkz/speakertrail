package api

import (
	"errors"
	"net/http"

	"github.com/timmrkz/speakertrail/internal/store"
)

// undo takes back the last change Tim made, from Undo in the Edit menu,
// see store.Edit. It answers what it changed, so the interface can show
// it, or done false when there was nothing to undo.
// An undo of the page whose paste started a draft stops the draft, see
// draft.Drafter.Undone, and one that took people out stops their drafts,
// since there is nobody left to keep them for.
func (a *API) undo(w http.ResponseWriter, r *http.Request) {
	u, err := a.Store.Undo()
	if err == nil && a.Drafts != nil {
		if u.Shows != nil && u.Shows.Kind == "person" {
			a.Drafts.Undone(u.Shows.ID)
		}
		for _, id := range u.Gone {
			// A draft that already ended has nothing to stop.
			_ = a.Drafts.Cancel(id)
		}
	}
	a.sendUndone(w, r, u, err)
}

// redo does again what the last undo took back.
func (a *API) redo(w http.ResponseWriter, r *http.Request) {
	u, err := a.Store.Redo()
	a.sendUndone(w, r, u, err)
}

func (a *API) sendUndone(w http.ResponseWriter, r *http.Request, u store.Undone, err error) {
	switch {
	case errors.Is(err, store.ErrChangedSince):
		fail(w, http.StatusConflict, capital(err.Error()))
	case err != nil:
		a.internal(w, r, err)
	default:
		send(w, http.StatusOK, u)
	}
}
