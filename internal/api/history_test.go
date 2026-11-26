package api

import (
	"net/http"
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
