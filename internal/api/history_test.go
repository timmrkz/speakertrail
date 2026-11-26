package api

import (
	"net/http"
	"slices"
	"testing"

	"github.com/timmrkz/speakertrail/internal/store"
)

// Connect and Not this person are taken back by Undo in the Edit menu,
// the last first, and done again by Redo, each answering whom it changed.
func TestUndoAndRedoChangesToAPerson(t *testing.T) {
	a, st, _ := newAPI(t)
	var id int64
	st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Keramik Köln", 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/mara-beispiel", Name: "Mara Beispiel"}, s, 1, t0)
		id = p.ID
		return nil
	})
	path := "/api/people/" + itoa(id)
	if code := call(t, a, "PATCH", path, `{"connected": true}`, nil); code != http.StatusOK {
		t.Fatalf("connect: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/profiles/"+itoa(id), `{"review": "rejected"}`, nil); code != http.StatusNoContent {
		t.Fatalf("reject: %d", code)
	}
	var p personDetail
	step := func(how string) store.Undone {
		t.Helper()
		var u store.Undone
		if code := call(t, a, "POST", "/api/"+how, "{}", &u); code != http.StatusOK {
			t.Fatalf("%s: %d", how, code)
		}
		call(t, a, "GET", path, "", &p)
		return u
	}
	if u := step("undo"); !u.Done || u.Shows == nil || *u.Shows != store.PersonThing(id) {
		t.Errorf("undo answered %+v, want the person", u)
	}
	if p.Profiles[0].Review != "confirmed" || !p.Connected {
		t.Errorf("after one undo: review %q, connected %v, want confirmed and connected", p.Profiles[0].Review, p.Connected)
	}
	step("undo")
	if p.Connected {
		t.Error("after two undos they are still connected")
	}
	if u := step("undo"); u.Done {
		t.Errorf("a third undo did %+v, want nothing", u)
	}
	step("redo")
	if !p.Connected || p.Profiles[0].Review != "confirmed" {
		t.Errorf("after redo: connected %v, review %q", p.Connected, p.Profiles[0].Review)
	}
}

// Notes changed since by something else are not written over.
func TestAnUndoOfWhatChangedSinceIsRefused(t *testing.T) {
	a, st, _ := newAPI(t)
	var id int64
	st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Keramik Köln", 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/mara-beispiel", Name: "Mara Beispiel"}, s, 1, t0)
		id = p.ID
		return nil
	})
	call(t, a, "PATCH", "/api/people/"+itoa(id), `{"notes": "Töpfert"}`, nil)
	st.Update(func(d *store.Data) error {
		d.Person(id).Notes = "changed elsewhere"
		return nil
	})
	var out struct {
		Error string `json:"error"`
	}
	if code := call(t, a, "POST", "/api/undo", "{}", &out); code != http.StatusConflict || out.Error == "" {
		t.Errorf("undo: %d %q, want 409 saying why", code, out.Error)
	}
}

// Searches, exclusions, the template and a setting are taken back by
// Undo in the Edit menu, the last first, each answering what it changed
// so the page opens it.
func TestUndoSearchesExclusionsTheTemplateAndSettings(t *testing.T) {
	a, _, _ := newAPI(t)
	var s searchJSON
	if code := call(t, a, "POST", "/api/sources", `{"query": "Keramik Köln"}`, &s); code != http.StatusCreated {
		t.Fatalf("add a search: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/sources/"+itoa(s.ID), `{"status": "retired"}`, nil); code != http.StatusOK {
		t.Fatalf("pause: %d", code)
	}
	var x exclusionJSON
	if code := call(t, a, "POST", "/api/exclusions", `{"phrase": "Hochzeitsplanung"}`, &x); code != http.StatusCreated {
		t.Fatalf("add an exclusion: %d", code)
	}
	if code := call(t, a, "DELETE", "/api/exclusions/"+itoa(x.ID), "", nil); code != http.StatusNoContent {
		t.Fatalf("remove the exclusion: %d", code)
	}
	if code := call(t, a, "PATCH", "/api/settings/profile_searches_per_run", `{"value": 12}`, nil); code != http.StatusOK {
		t.Fatalf("change a setting: %d", code)
	}
	if code := call(t, a, "PUT", "/api/template/parts/greeting", `{"variations": ["Moin"]}`, nil); code != http.StatusOK {
		t.Fatalf("change a part: %d", code)
	}

	undo := func(want store.Thing) {
		t.Helper()
		var u store.Undone
		if code := call(t, a, "POST", "/api/undo", "{}", &u); code != http.StatusOK || !u.Done || u.Shows == nil || *u.Shows != want {
			t.Fatalf("undo: %d %+v, want %+v", code, u, want)
		}
	}
	var tpl templateJSON
	undo(store.TemplateThing())
	call(t, a, "GET", "/api/template", "", &tpl)
	if tpl.Changed {
		t.Error("after undo the template is still changed")
	}
	undo(store.SettingThing("profile_searches_per_run"))
	var set store.Setting
	var all struct {
		Settings []store.Setting `json:"settings"`
	}
	call(t, a, "GET", "/api/settings", "", &all)
	for _, v := range all.Settings {
		if v.Key == "profile_searches_per_run" {
			set = v
		}
	}
	if string(set.Value) != "10" {
		t.Errorf("after undo the setting is %s, want 10", set.Value)
	}
	undo(store.ExclusionThing(x.ID))
	var xs []exclusionJSON
	call(t, a, "GET", "/api/exclusions", "", &xs)
	if !slices.ContainsFunc(xs, func(e exclusionJSON) bool { return e.ID == x.ID && e.Phrase == "Hochzeitsplanung" }) {
		t.Errorf("after undo the exclusions are %+v, want Hochzeitsplanung back with its id", xs)
	}
	undo(store.ExclusionThing(x.ID))
	undo(store.SearchThing(s.ID))
	undo(store.SearchThing(s.ID))
	var list struct {
		Sources []searchJSON `json:"sources"`
	}
	call(t, a, "GET", "/api/sources", "", &list)
	if slices.ContainsFunc(list.Sources, func(o searchJSON) bool { return o.ID == s.ID }) {
		t.Error("after undoing the add the search is still there")
	}
	var r store.Undone
	call(t, a, "POST", "/api/redo", "{}", &r)
	call(t, a, "GET", "/api/sources", "", &list)
	if !slices.ContainsFunc(list.Sources, func(o searchJSON) bool { return o.ID == s.ID && o.Status == "candidate" }) {
		t.Errorf("after redo the searches are %+v, want Keramik Köln back with its id", list.Sources)
	}
}

// The template is no longer put in place whole. Undo does that.
func TestThereIsNoPutOfAWholeTemplate(t *testing.T) {
	a, _, _ := newAPI(t)
	if code := call(t, a, "PUT", "/api/template", `{"parts": []}`, nil); code != http.StatusMethodNotAllowed && code != http.StatusNotFound {
		t.Errorf("PUT /api/template answered %d, want it gone", code)
	}
}
