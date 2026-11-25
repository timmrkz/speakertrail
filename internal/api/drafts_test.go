package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/draft"
	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/store"
)

// A first message is drafted, shows on the person, and keeps what Tim
// changed in it.
func TestDraftAFirstMessage(t *testing.T) {
	a, st, _ := newAPI(t)
	var id int64
	st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Keramik Köln", 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/mara-beispiel", Name: "Mara Beispiel",
			Text: "# Mara Beispiel\n\n## About\n\nKeramik"}, s, 1, t0)
		id = p.ID
		return nil
	})
	if code := call(t, a, "POST", "/api/people/"+itoa(id)+"/draft", "{}", nil); code != http.StatusConflict {
		t.Errorf("without drafting: %d", code)
	}
	host := &language.Host{Launch: func(context.Context, string) (*language.Server, error) { return &language.Server{}, nil }}
	t.Cleanup(host.Close)
	m := language.Models()[0]
	a.Drafts = &draft.Drafter{Store: st, Host: host,
		Model: func() (language.Model, string, error) { return m, "/models/" + m.Name, nil },
		Ask: func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
			return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, Finish: "stop"}, nil
		},
		Pick: func(int) int { return 0 },
	}
	// Without a model, Draft is refused and says why.
	a.Drafts.Model = func() (language.Model, string, error) { return language.Model{}, "", draft.ErrNoModel }
	var refused map[string]string
	if code := call(t, a, "POST", "/api/people/"+itoa(id)+"/draft", "{}", &refused); code != http.StatusConflict ||
		!strings.HasPrefix(refused["error"], "Download a language model in Settings first") {
		t.Errorf("without a model: %d %v", code, refused)
	}
	a.Drafts.Model = func() (language.Model, string, error) { return m, "/models/" + m.Name, nil }
	var j draft.Job
	if code := call(t, a, "POST", "/api/people/"+itoa(id)+"/draft", "{}", &j); code != http.StatusOK || j.PersonID != id {
		t.Fatalf("start: %d %+v", code, j)
	}
	a.Drafts.Wait()
	var jobs []draft.Job
	call(t, a, "GET", "/api/drafts", "", &jobs)
	if len(jobs) != 1 || jobs[0].State != "done" {
		t.Errorf("jobs %+v", jobs)
	}
	var p personDetail
	call(t, a, "GET", "/api/people/"+itoa(id), "", &p)
	if !strings.HasPrefix(p.Draft, "Hallo Mara,\n\nsuper cool mit dir zu connecten. Du bist mir in meinem LinkedIn Feed aufgefallen, weil du töpferst.\n") ||
		!strings.Contains(p.Draft, "als Töpferin zu erzählen") || p.DraftedAt == nil || p.ProfileReadAt == nil {
		t.Errorf("person %+v", p)
	}
	call(t, a, "PATCH", "/api/people/"+itoa(id), `{"draft":"Hallo Mara, danke!"}`, &p)
	if p.Draft != "Hallo Mara, danke!" || p.DraftedAt == nil {
		t.Errorf("edited draft %q drafted at %v", p.Draft, p.DraftedAt)
	}
	// An emptied message was drafted at no time, so the person page says
	// nothing was drafted over an empty box (review of #60, 22 November
	// 2026).
	call(t, a, "PATCH", "/api/people/"+itoa(id), `{"draft":""}`, &p)
	if p.Draft != "" || p.DraftedAt != nil {
		t.Errorf("emptied draft %q drafted at %v", p.Draft, p.DraftedAt)
	}
	// Undo in the Edit menu takes each back, the drafted message too.
	for _, want := range []string{"Hallo Mara, danke!", "Hallo Mara,\n\nsuper cool", ""} {
		call(t, a, "POST", "/api/undo", "{}", nil)
		call(t, a, "GET", "/api/people/"+itoa(id), "", &p)
		if !strings.HasPrefix(p.Draft, want) || (want == "") != (p.DraftedAt == nil) {
			t.Errorf("after undo the draft is %q drafted at %v, want %q", p.Draft, p.DraftedAt, want)
		}
	}
	if code := call(t, a, "POST", "/api/people/"+itoa(id)+"/draft/cancel", "{}", nil); code != http.StatusConflict {
		t.Errorf("cancel with nothing under way: %d", code)
	}
	if code := call(t, a, "POST", "/api/people/999/draft", "{}", nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
	}
}

// A LinkedIn page pasted with nobody open finds whose it is by the links
// Chrome copied with it, keeps it as their profile, and drafts the first
// message of someone connected who has none (Tim's report of 24
// November 2026, #88 and #89). Undo in the Edit menu takes back the
// draft, then the paste. The paste that added someone is no step of its
// own: the toast's Undo takes them out.
func TestAPastedPageFindsItsPersonAndDrafts(t *testing.T) {
	a, st, e := newAPI(t)
	var added struct{ New []int64 }
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/mara-s-beispiel"}`, &added)
	e.Wait()
	id := added.New[0]
	host := &language.Host{Launch: func(context.Context, string) (*language.Server, error) { return &language.Server{}, nil }}
	t.Cleanup(host.Close)
	m := language.Models()[0]
	a.Drafts = &draft.Drafter{Store: st, Host: host,
		Model: func() (language.Model, string, error) { return m, "/models/" + m.Name, nil },
		Ask: func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
			return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, Finish: "stop"}, nil
		},
		Pick: func(int) int { return 0 },
	}
	page := "Home\nMara Beispiel\n· 1st\nGründerin | Probe Keramik\nDortmund, North Rhine-Westphalia, Germany\nAbout\nIch töpfere."
	body, _ := json.Marshal(map[string]any{"text": page, "links": []string{"/in/mara-s-beispiel/overlay/contact-info/", "/in/mara-s-beispiel/details/"}})
	var out struct {
		Person   personDetail
		Named    bool
		Drafting bool
	}
	if code := call(t, a, "POST", "/api/people/profile", string(body), &out); code != http.StatusOK || out.Person.ID != id || !out.Named || !out.Drafting {
		t.Fatalf("paste: %d %+v", code, out)
	}
	a.Drafts.Wait()
	var p personDetail
	call(t, a, "GET", "/api/people/"+itoa(id), "", &p)
	if p.Name != "Mara Beispiel" || !strings.Contains(p.Draft, "weil du töpferst.") {
		t.Errorf("person %q, draft %q", p.Name, p.Draft)
	}
	for _, what := range []string{"the draft", "the paste"} {
		var u store.Undone
		if code := call(t, a, "POST", "/api/undo", "{}", &u); code != http.StatusOK || !u.Done {
			t.Fatalf("undo of %s: %d %+v", what, code, u)
		}
	}
	call(t, a, "GET", "/api/people/"+itoa(id), "", &p)
	if p.Draft != "" || p.ProfileReadAt != nil || p.Name == "Mara Beispiel" {
		t.Errorf("after undo: person %q, draft %q, profile read at %v", p.Name, p.Draft, p.ProfileReadAt)
	}
	// The page of someone new adds them under the link in it, connected
	// as it says, drafts their first message, and Undo takes them out.
	newPage, _ := json.Marshal(map[string]any{"text": "Ida Neu\n· 1st\nCoach in Bonn\nAbout\nIch coache.", "links": []string{"/in/ida-neu-9/", "/in/ida-neu-9/details/"}, "open": id})
	if code := call(t, a, "POST", "/api/people/profile", string(newPage), &out); code != http.StatusOK || out.Person.ID == id || out.Person.Name != "Ida Neu" || !out.Person.Connected || !out.Drafting || !out.Named {
		t.Fatalf("someone new: %d %+v", code, out)
	}
	a.Drafts.Wait()
	// Undo in the Edit menu takes back their draft, and nothing more.
	for _, want := range []bool{true, false} {
		var u store.Undone
		if code := call(t, a, "POST", "/api/undo", "{}", &u); code != http.StatusOK || u.Done != want {
			t.Fatalf("undo after adding: %d %+v, want done %v", code, u, want)
		}
	}
	if code := call(t, a, "DELETE", "/api/people/"+itoa(out.Person.ID), "", nil); code != http.StatusNoContent {
		t.Errorf("undo someone new: %d", code)
	}
	// Pasted as text alone, Exa is asked for the link by the name and
	// headline, and knows only another name, so it says what is missing.
	var refused map[string]string
	other, _ := json.Marshal(map[string]any{"text": "Ida Neu\n· 2nd\nCoach"})
	if code := call(t, a, "POST", "/api/people/profile", string(other), &refused); code != http.StatusUnprocessableEntity || !strings.HasPrefix(refused["error"], "The page came without the link to their profile") {
		t.Errorf("no link: %d %v", code, refused)
	}
	if code := call(t, a, "POST", "/api/people/profile", `{"text":" "}`, nil); code != http.StatusBadRequest {
		t.Errorf("an empty paste: %d", code)
	}
}
