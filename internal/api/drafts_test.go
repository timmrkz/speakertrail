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
