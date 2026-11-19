package api

import (
	"net/http"

	"github.com/timmrkz/speakertrail/internal/draft"
)

// The first messages the language model drafts on the Mac, see
// internal/draft. A draft runs in the background, and how far it is comes
// to the window as the event draft, the job as GET /api/drafts gives it.

// errNoDrafts is drafting asked for where no language model can run.
const errNoDrafts = "Drafting needs the Mac app"

func (a *API) drafts(w http.ResponseWriter, r *http.Request) {
	if a.Drafts == nil {
		send(w, http.StatusOK, []draft.Job{})
		return
	}
	send(w, http.StatusOK, a.Drafts.Jobs())
}

func (a *API) startDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if a.Drafts == nil {
		fail(w, http.StatusConflict, errNoDrafts)
		return
	}
	j, err := a.Drafts.Start(id)
	if err != nil {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	send(w, http.StatusOK, j)
}

func (a *API) cancelDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if a.Drafts == nil {
		fail(w, http.StatusConflict, errNoDrafts)
		return
	}
	if err := a.Drafts.Cancel(id); err != nil {
		fail(w, http.StatusConflict, "No draft is under way for them")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
