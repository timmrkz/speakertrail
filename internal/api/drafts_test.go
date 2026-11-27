package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/timmrkz/speakertrail/internal/draft"
	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/search"
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

// Undo in the Edit menu of the page whose paste started a draft stops
// the draft, as the paste's Undo toast did. An undo of the notes typed
// meanwhile leaves it running.
func TestUndoOfAPasteStopsTheDraftItStarted(t *testing.T) {
	a, st, e := newAPI(t)
	var added struct{ New []int64 }
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/mara-s-beispiel"}`, &added)
	e.Wait()
	id := added.New[0]
	host := &language.Host{Launch: func(context.Context, string) (*language.Server, error) { return &language.Server{}, nil }}
	t.Cleanup(host.Close)
	m := language.Models()[0]
	asked := make(chan context.Context, 1)
	a.Drafts = &draft.Drafter{Store: st, Host: host,
		Model: func() (language.Model, string, error) { return m, "/models/" + m.Name, nil },
		// The model writes until the draft is stopped.
		Ask: func(ctx context.Context, _ *language.Server, _ []language.Message, _ json.RawMessage, _ func(string)) (language.Answer, error) {
			asked <- ctx
			<-ctx.Done()
			return language.Answer{}, ctx.Err()
		},
		Pick: func(int) int { return 0 },
	}
	page := "Home\nMara Beispiel\n· 1st\nGründerin | Probe Keramik\nDortmund, North Rhine-Westphalia, Germany\nAbout\nIch töpfere."
	body, _ := json.Marshal(map[string]any{"text": page, "links": []string{"/in/mara-s-beispiel/details/"}})
	var out struct{ Drafting bool }
	if code := call(t, a, "POST", "/api/people/profile", string(body), &out); code != http.StatusOK || !out.Drafting {
		t.Fatalf("paste: %d %+v", code, out)
	}
	writing := <-asked
	// Whatever the test finds, the draft ends with it.
	t.Cleanup(func() { _ = a.Drafts.Cancel(id); a.Drafts.Wait() })
	call(t, a, "PATCH", "/api/people/"+itoa(id), `{"notes":"Töpfert"}`, nil)
	call(t, a, "POST", "/api/undo", "{}", nil)
	if writing.Err() != nil {
		t.Error("the undo of the notes stopped the draft")
	}
	// The page, from two requests at once: one takes it back, the other
	// finds nothing left.
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { call(t, a, "POST", "/api/undo", "{}", nil) })
	}
	wg.Wait()
	// Cancel stops it before the undo answers.
	if writing.Err() == nil {
		t.Fatal("the undo of the page left the draft running")
	}
	a.Drafts.Wait()
	var p personDetail
	call(t, a, "GET", "/api/people/"+itoa(id), "", &p)
	if p.Draft != "" || p.ProfileReadAt != nil || p.Notes != "" {
		t.Errorf("after undo: draft %q, profile read at %v, notes %q", p.Draft, p.ProfileReadAt, p.Notes)
	}
}

// countedFinder is the invented index of finder, holding every read of a
// profile until gate is closed and counting the reads of each link.
type countedFinder struct {
	finder
	gate chan struct{}
	mu   sync.Mutex
	read map[string]int
}

func (f *countedFinder) Profile(ctx context.Context, link string) (search.Profile, error) {
	f.mu.Lock()
	f.read[link]++
	f.mu.Unlock()
	select {
	case <-f.gate:
	case <-ctx.Done():
		return search.Profile{}, ctx.Err()
	}
	return f.finder.Profile(ctx, link)
}

func (f *countedFinder) reads(link string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read[link]
}

// newDraftingAPI is an API that drafts, with Exa's reads held at gate
// and every state of a draft told on states.
func newDraftingAPI(t *testing.T) (*API, *engine.Engine, *countedFinder, chan draft.Job) {
	t.Helper()
	a, st, _ := newAPI(t)
	f := &countedFinder{gate: make(chan struct{}), read: map[string]int{}}
	e := &engine.Engine{Store: st, Finder: func() (engine.Finder, error) { return f, nil }}
	a.Engine = e
	host := &language.Host{Launch: func(context.Context, string) (*language.Server, error) { return &language.Server{}, nil }}
	t.Cleanup(host.Close)
	m := language.Models()[0]
	states := make(chan draft.Job, 256)
	a.Drafts = &draft.Drafter{Store: st, Host: host, Await: e.AwaitRead,
		Model:  func() (language.Model, string, error) { return m, "/models/" + m.Name, nil },
		Finder: func() (draft.Finder, error) { return f, nil },
		Ask: func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
			return language.Answer{Text: `{"reason":"du ein Studio hast.","reaction":"","portrait":"Inhaberin"}`, Finish: "stop"}, nil
		},
		Pick: func(int) int { return 0 },
		Told: func(j draft.Job) { states <- j },
	}
	return a, e, f, states
}

// reading waits until the draft reads the profile, and so waits on the
// run, or asks Exa itself.
func reading(t *testing.T, states chan draft.Job) {
	t.Helper()
	for j := range states {
		if j.State == "reading" {
			return
		}
		if j.Ended() {
			t.Fatalf("the draft ended before it read: %+v", j)
		}
	}
}

// One link that adds one new person opens them and starts their first
// message at once (Tim, 27 November 2026, #115). The draft waits for the
// run's read of the profile rather than ask Exa for it a second time,
// and greets the name the read found.
func TestOneLinkOpensThePersonAndDrafts(t *testing.T) {
	a, e, f, states := newDraftingAPI(t)
	var added struct {
		New      []int64
		Open     int64
		Drafting bool
	}
	if code := call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/ida-probe"}`, &added); code != http.StatusOK ||
		len(added.New) != 1 || added.Open != added.New[0] || !added.Drafting {
		t.Fatalf("add: %d %+v", code, added)
	}
	reading(t, states)
	close(f.gate)
	e.Wait()
	a.Drafts.Wait()
	if n := f.reads("https://www.linkedin.com/in/ida-probe"); n != 1 {
		t.Errorf("Exa was asked %d times for the profile, want once", n)
	}
	var p personDetail
	call(t, a, "GET", "/api/people/"+itoa(added.Open), "", &p)
	if p.Name != "Ida Probe" || !strings.HasPrefix(p.Draft, "Hallo Ida,") || !strings.Contains(p.Draft, "weil du ein Studio hast.") {
		t.Errorf("person %q, draft %q", p.Name, p.Draft)
	}
}

// A link Exa's index lacks ends the draft with what to do, without asking
// Exa again for what the run just found missing.
func TestOneLinkNotInTheIndexAsksExaOnce(t *testing.T) {
	a, e, f, states := newDraftingAPI(t)
	var added struct {
		Open     int64
		Drafting bool
	}
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/neu-hier-3"}`, &added)
	if added.Open == 0 || !added.Drafting {
		t.Fatalf("add: %+v", added)
	}
	reading(t, states)
	close(f.gate)
	e.Wait()
	a.Drafts.Wait()
	if n := f.reads("https://www.linkedin.com/in/neu-hier-3"); n != 1 {
		t.Errorf("Exa was asked %d times for the profile, want once", n)
	}
	var jobs []draft.Job
	call(t, a, "GET", "/api/drafts", "", &jobs)
	if len(jobs) != 1 || jobs[0].State != "failed" || jobs[0].Error != draft.Sentence(draft.ErrNoCopy.Error()) || !jobs[0].NoCopy {
		t.Errorf("jobs %+v", jobs)
	}
}

// Several links, or one new person with any other link, land on People
// under New as before, and start no draft. Without the Exa key the one
// person still opens, and no draft starts, since their profile waits
// for the next run.
func TestManyLinksOpenNobodyAndDraftNothing(t *testing.T) {
	a, _, f, _ := newDraftingAPI(t)
	close(f.gate)
	for _, text := range []string{
		"https://www.linkedin.com/in/ida-probe linkedin.com/in/neu-hier-3",
		"https://www.linkedin.com/in/mara-probe https://www.linkedin.com/company/probe-gmbh",
		"https://www.linkedin.com/in/ida-probe linkedin.com/in/lea-probe",
	} {
		var added struct {
			Open     int64
			Drafting bool
		}
		if code := call(t, a, "POST", "/api/people", `{"text":"`+text+`"}`, &added); code != http.StatusOK || added.Open != 0 || added.Drafting {
			t.Errorf("%s: %d %+v", text, code, added)
		}
	}
	a.Engine.Wait()
	a.Drafts.Wait()
	if jobs := a.Drafts.Jobs(); len(jobs) != 0 {
		t.Errorf("drafts %+v", jobs)
	}
	a.Engine.Finder = func() (engine.Finder, error) { return nil, engine.ErrNoKey }
	var added struct {
		New      []int64
		Open     int64
		Drafting bool
		Wait     string
	}
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/ohne-schluessel"}`, &added)
	if len(added.New) != 1 || added.Open != added.New[0] || added.Drafting || added.Wait == "" {
		t.Errorf("without a key: %+v", added)
	}
}

// Undo of the link, from several goroutines at once, takes the person
// out and stops their draft while it waits for the run, and the run
// keeps nothing of them.
func TestUndoOfOneLinkStopsItsDraft(t *testing.T) {
	a, e, f, states := newDraftingAPI(t)
	var added struct {
		Open     int64
		Drafting bool
	}
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/ida-probe"}`, &added)
	if !added.Drafting {
		t.Fatalf("add: %+v", added)
	}
	reading(t, states)
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for range 4 {
		wg.Go(func() { codes <- call(t, a, "DELETE", "/api/people/"+itoa(added.Open), "", nil) })
	}
	wg.Wait()
	close(codes)
	removed := 0
	for c := range codes {
		if c == http.StatusNoContent {
			removed++
		}
	}
	if removed != 1 {
		t.Errorf("%d of the undos took them out, want 1", removed)
	}
	a.Drafts.Wait()
	if jobs := a.Drafts.Jobs(); len(jobs) != 1 || jobs[0].State != "cancelled" {
		t.Errorf("jobs %+v", jobs)
	}
	close(f.gate)
	e.Wait()
	if code := call(t, a, "GET", "/api/people/"+itoa(added.Open), "", nil); code != http.StatusNotFound {
		t.Errorf("after undo: %d", code)
	}
}
