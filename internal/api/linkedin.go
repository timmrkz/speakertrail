package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/timmrkz/speakertrail/internal/browser"
	"github.com/timmrkz/speakertrail/internal/store"
)

// Browsers is what the API needs from the Mac to open LinkedIn in the
// right browser profile, see internal/browser. Tests hand it a fake.
type Browsers interface {
	Profiles(app string) ([]browser.Profile, error)
	Open(ctx context.Context, app, folder, link string) error
}

// Mac opens links with internal/browser in the browsers on this Mac.
type Mac struct{ Support string }

func (m Mac) Profiles(app string) ([]browser.Profile, error) { return browser.Profiles(m.Support, app) }

func (m Mac) Open(ctx context.Context, app, folder, link string) error {
	return browser.Open(ctx, m.Support, app, folder, link)
}

type browserJSON struct {
	App      string            `json:"app"`
	Profiles []browser.Profile `json:"profiles"`
}

// browsers lists every Chromium browser on this Mac with its profiles,
// read anew each time.
func (a *API) browsers() []browserJSON {
	found := []browserJSON{}
	if a.Browsers == nil {
		return found
	}
	for _, b := range browser.Apps {
		ps, err := a.Browsers.Profiles(b.App)
		if err != nil || len(ps) == 0 {
			continue
		}
		found = append(found, browserJSON{App: b.App, Profiles: ps})
	}
	return found
}

// opensIn is where LinkedIn opens: the profile Tim chose, or else the
// only profile of the only browser on this Mac. Empty when it is neither,
// because guessing is the coin toss this avoids.
func opensIn(chosen store.Where, found []browserJSON) store.Where {
	if chosen.Folder == "" && len(found) == 1 && len(found[0].Profiles) == 1 {
		return store.Where{App: found[0].App, Folder: found[0].Profiles[0].Folder}
	}
	return chosen
}

func (a *API) chosen() store.Where {
	var w store.Where
	a.Store.Read(func(d *store.Data) { w = d.LinkedInOpens })
	return w
}

// linkedin answers where LinkedIn opens and the choices there are.
func (a *API) linkedin(w http.ResponseWriter, r *http.Request) {
	found := a.browsers()
	at := opensIn(a.chosen(), found)
	// Whether the profile is still there, so Settings can say so rather
	// than Chrome quietly making a new one.
	known := false
	for _, b := range found {
		for _, p := range b.Profiles {
			known = known || (b.App == at.App && p.Folder == at.Folder)
		}
	}
	send(w, http.StatusOK, map[string]any{"app": at.App, "folder": at.Folder, "known": known, "browsers": found})
}

// setLinkedin chooses the browser profile LinkedIn opens in. Only a
// profile that is there now can be chosen, so the choice never names a
// folder Chrome would quietly make anew.
func (a *API) setLinkedin(w http.ResponseWriter, r *http.Request) {
	var in store.Where
	if !decode(w, r, &in) {
		return
	}
	if a.Browsers == nil {
		fail(w, http.StatusConflict, "This app cannot open a browser")
		return
	}
	ps, err := a.Browsers.Profiles(in.App)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !slices.ContainsFunc(ps, func(p browser.Profile) bool { return p.Folder == in.Folder }) {
		fail(w, http.StatusBadRequest, "There is no such profile in "+in.App)
		return
	}
	if !a.update(w, r, func(d *store.Data) error {
		d.LinkedInOpens = in
		return nil
	}) {
		return
	}
	a.linkedin(w, r)
}

// openLinkedin opens a person's LinkedIn profile where LinkedIn opens.
// The interface names the person, and the link comes from the file, so
// nothing but a profile a search kept is ever opened.
func (a *API) openLinkedin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var link string
	found := false
	a.Store.Read(func(d *store.Data) {
		if p := d.Person(id); p != nil {
			found = true
			if p.LinkedIn.Review != "rejected" {
				link = p.LinkedIn.URL
			}
		}
	})
	switch {
	case !found:
		fail(w, http.StatusNotFound, "Not found")
		return
	case link == "":
		fail(w, http.StatusNotFound, "This person has no LinkedIn profile")
		return
	}
	if err := browser.CheckLink(link); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.Browsers == nil {
		fail(w, http.StatusConflict, "This app cannot open a browser")
		return
	}
	at := opensIn(a.chosen(), a.browsers())
	if at.Folder == "" {
		choose(w, "Choose in Settings where LinkedIn opens")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Browsers.Open(ctx, at.App, at.Folder, link); err != nil {
		if errors.Is(err, browser.ErrNoBrowser) {
			choose(w, at.App+" is not on this Mac. Choose another in Settings")
			return
		}
		fail(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// choose answers that LinkedIn cannot open until Tim chooses where in
// Settings. The interface goes by choose, never by the words.
func choose(w http.ResponseWriter, msg string) {
	send(w, http.StatusConflict, map[string]any{"error": msg, "choose": true})
}
