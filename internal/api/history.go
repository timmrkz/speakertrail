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
// draft.Drafter.Undone.
func (a *API) undo(w http.ResponseWriter, r *http.Request) {
	u, err := a.Store.Undo()
	if err == nil && u.Shows != nil && u.Shows.Kind == "person" && a.Drafts != nil {
		a.Drafts.Undone(u.Shows.ID)
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
