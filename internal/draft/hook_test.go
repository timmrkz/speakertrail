package draft

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/store"
)

func TestTheHookIsHeldToTheSchema(t *testing.T) {
	long := strings.Repeat("x", 121)
	for _, c := range []struct{ answer, says string }{
		{`{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, ""},
		{"\n " + `{"reason":"du töpferst.","reaction":"Toll!","portrait":"Töpferin"}` + " \n", ""},
		{`{"reason":"` + strings.Repeat("ö", 120) + `","reaction":"` + strings.Repeat("x", 60) + `","portrait":"Ab"}`, ""},
		{`{"reason":"` + long + `","reaction":"","portrait":"Töpferin"}`, "reason has 121 letters, at most 120"},
		{`{"reason":"du","reaction":"","portrait":"Töpferin"}`, "reason has 2 letters, at least 5"},
		{`{"reason":"du töpferst.","reaction":"` + strings.Repeat("x", 61) + `","portrait":"Töpferin"}`, "reaction has 61 letters"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"` + strings.Repeat("x", 41) + `"}`, "portrait has 41 letters"},
		{`{"reason":"du töpferst.","reaction":""}`, "left out portrait"},
		{`{"reason":"du töpferst.","portrait":"Töpferin"}`, "left out reaction"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Töpferin","extra":"x"}`, "a field the schema does not know"},
		{`{"reason":1,"reaction":"","portrait":"Töpferin"}`, "reason is not text"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"} und mehr`, "more after the JSON"},
		{`{"reason":"du töpferst.","reac`, "stops before the JSON ends"},
		{`Hallo Mara, schön dich zu sehen!`, "not JSON"},
		{``, "empty"},
	} {
		_, err := ParseHook(c.answer)
		if (c.says == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.says)) {
			t.Errorf("%.60q: %v, want %q", c.answer, err, c.says)
		}
	}
}

// What the model writes goes in the middle of a sentence, so it comes on
// one line, without characters that cannot be written.
func TestTheHookComesOnOneLine(t *testing.T) {
	h, err := ParseHook(`{"reason":"  du\n\ntöpferst\u200b\u0007 in\tKöln! 👩‍🎨 ","reaction":"","portrait":"Töpfer\ufeffin"}`)
	if err != nil {
		t.Fatal(err)
	}
	if h.Reason != "du töpferst in Köln! 👩‍🎨" || h.Portrait != "Töpferin" {
		t.Errorf("got %q and %q", h.Reason, h.Portrait)
	}
}

// An answer that does not fit still makes a draft, with … for Tim to
// write and the window told why, never what the model wrote.
func TestAnAnswerThatDoesNotFitStillDrafts(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		return language.Answer{Text: "Liebe Mara, du bist toll!", Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	j := r.d.Jobs()[0]
	want := "Hallo Mara, du bist mir aufgefallen, weil …\n\nDeine Geschichte als …?"
	if j.State != "done" || j.Text != want || !strings.Contains(j.Problem, "did not fit") || strings.Contains(j.Problem, "toll") {
		t.Errorf("the job: %+v", j)
	}
	r.st.Read(func(d *store.Data) {
		if p := d.Person(id); p.Draft != want {
			t.Errorf("kept %q", p.Draft)
		}
	})
	// The next draft that fits says nothing of the one before.
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	if j := r.d.Jobs()[0]; j.Problem != "" || !strings.Contains(j.Text, "weil du töpferst.\n") {
		t.Errorf("the next job: %+v", j)
	}
}

// What the model writes is put in as it is: a #part# in it is not put
// together, and markup stays text for the window to show as text.
func TestWhatTheModelWritesStaysText(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		return language.Answer{Text: `{"reason":"du #greeting# <img src=x onerror=alert(1)>.","reaction":"","portrait":"#origin#"}`, Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	want := "Hallo Mara, du bist mir aufgefallen, weil du #greeting# <img src=x onerror=alert(1)>.\n\nDeine Geschichte als #origin#?"
	if j := r.d.Jobs()[0]; j.Text != want {
		t.Errorf("got %q", j.Text)
	}
}

// A template that cannot be used, in a file from somewhere else, fails
// the draft with what is wrong, before the model is asked.
func TestABrokenTemplateFailsTheDraft(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	r.st.Update(func(d *store.Data) error {
		d.Template = &store.Template{Parts: []store.Part{{Name: "origin", Variations: []string{"#nope#"}}}}
		return nil
	})
	r.d.Start(id)
	r.d.Wait()
	if j := r.d.Jobs()[0]; j.State != "failed" || !strings.Contains(j.Error, "template in Settings") || r.asks.Load() != 0 {
		t.Errorf("the job: %+v, %d asks", j, r.asks.Load())
	}
}

// Every variation can come up: the picks reach each one, and a draft
// without a Pick of its own picks at random.
func TestDraftsPickAtRandom(t *testing.T) {
	r := newRig(t)
	r.d.Pick = nil
	seen := map[string]bool{}
	for range 200 {
		seen[string(r.d.put(store.DefaultTemplate(), "Mara Beispiel", ""))[:40]] = true
	}
	if len(seen) < 5 {
		t.Errorf("200 drafts began %d ways", len(seen))
	}
}
