package api

import (
	"context"
	"encoding/json"
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

// linkedin answers where LinkedIn opens and the choices there are: every
// Chromium browser on this Mac with its profiles, read anew each time.
// One browser with one profile is chosen without asking.
func (a *API) linkedin(w http.ResponseWriter, r *http.Request) {
	var app, folder string
	a.Store.Read(func(d *store.Data) {
		app, folder = d.Text("linkedin_browser"), d.Text("linkedin_profile")
	})
	found := []browserJSON{}
	if a.Browsers != nil {
		for _, b := range browser.Apps {
			ps, err := a.Browsers.Profiles(b.App)
			if err != nil || len(ps) == 0 {
				continue
			}
			found = append(found, browserJSON{App: b.App, Profiles: ps})
		}
	}
	if folder == "" && len(found) == 1 && len(found[0].Profiles) == 1 {
		app, folder = found[0].App, found[0].Profiles[0].Folder
	}
	// Whether the kept profile is still there, so Settings can say so
	// rather than Chrome quietly making a new one.
	known := false
	for _, b := range found {
		for _, p := range b.Profiles {
			known = known || (b.App == app && p.Folder == folder)
		}
	}
	send(w, http.StatusOK, map[string]any{"app": app, "folder": folder, "known": known, "browsers": found})
}

// setLinkedin chooses the browser profile LinkedIn opens in. Only a
// profile that is there now can be chosen, so the choice never names a
// folder Chrome would quietly make anew.
func (a *API) setLinkedin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		App    string `json:"app"`
		Folder string `json:"folder"`
	}
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
	app, _ := json.Marshal(in.App)
	folder, _ := json.Marshal(in.Folder)
	if !a.update(w, r, func(d *store.Data) error {
		now := a.Store.Now()
		if err := d.SetSetting("linkedin_browser", app, now); err != nil {
			return errBad{err}
		}
		if err := d.SetSetting("linkedin_profile", folder, now); err != nil {
			return errBad{err}
		}
		return nil
	}) {
		return
	}
	a.linkedin(w, r)
}

// open opens a LinkedIn profile in the chosen browser profile. Without a
// choice it does not guess, which is the coin toss this avoids.
func (a *API) open(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := browser.CheckLink(in.URL); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var app, folder string
	a.Store.Read(func(d *store.Data) {
		app, folder = d.Text("linkedin_browser"), d.Text("linkedin_profile")
	})
	if a.Browsers == nil {
		fail(w, http.StatusConflict, "This app cannot open a browser")
		return
	}
	if folder == "" {
		if ps, err := a.Browsers.Profiles(app); err == nil && len(ps) == 1 {
			folder = ps[0].Folder
		}
	}
	if folder == "" {
		fail(w, http.StatusConflict, "Choose in Settings where LinkedIn opens")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Browsers.Open(ctx, app, folder, in.URL); err != nil {
		if errors.Is(err, browser.ErrNoBrowser) {
			fail(w, http.StatusConflict, app+" is not on this Mac. Choose another in Settings")
			return
		}
		fail(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
