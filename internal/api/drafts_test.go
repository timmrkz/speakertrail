package api

import (
	"context"
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
		Ask: func(_ context.Context, _ *language.Server, _ []language.Message, _ func(string)) (language.Answer, error) {
			return language.Answer{Text: "Hallo Mara, danke fürs Vernetzen!", Finish: "stop"}, nil
		},
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
	if p.Draft != "Hallo Mara, danke fürs Vernetzen!" || p.DraftedAt == nil || p.ProfileReadAt == nil {
		t.Errorf("person %+v", p)
	}
	call(t, a, "PATCH", "/api/people/"+itoa(id), `{"draft":"Hallo Mara, danke!"}`, &p)
	if p.Draft != "Hallo Mara, danke!" {
		t.Errorf("edited draft %q", p.Draft)
	}
	if code := call(t, a, "POST", "/api/people/"+itoa(id)+"/draft/cancel", "{}", nil); code != http.StatusConflict {
		t.Errorf("cancel with nothing under way: %d", code)
	}
	if code := call(t, a, "POST", "/api/people/999/draft", "{}", nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
	}
}
