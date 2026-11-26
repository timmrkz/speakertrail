package draft

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/store"
)

func TestTheHookIsHeldToTheSchema(t *testing.T) {
	long := strings.Repeat("x", 121)
	for _, c := range []struct{ answer, says string }{
		{`{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, ""},
		{"\n " + `{"reason":"du töpferst.","reaction":"Toll!","portrait":"Töpferin"}` + " \n", ""},
		{`{"reason":"` + strings.Repeat("ö", 119) + `","reaction":"` + strings.Repeat("x", 59) + `","portrait":"Ab"}`, ""},
		// The grammar ends a text at its maxLength, so a text that long
		// was cut off, as the portrait of Tim's report was, at exactly 40.
		{`{"reason":"` + strings.Repeat("ö", 120) + `","reaction":"","portrait":"Töpferin"}`, "reason was cut off at the 120 letters"},
		{`{"reason":"du töpferst.","reaction":"` + strings.Repeat("x", 60) + `","portrait":"Töpferin"}`, "reaction was cut off at the 60 letters"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Unfallchirurgin und Sportmedizinerinnen"}`, ""},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Unfallchirurgin und Sportmedizinerinnenn"}`, "portrait was cut off at the 40 letters"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Arzt und G` + strings.Repeat(" ", 30) + `"}`, "portrait was cut off"},
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
		// Tim's report of 26 November 2026: a letter of another script in
		// the middle of a German sentence, in a portrait that ran on.
		{`{"reason":"du die Brücke zwischen Medizin und Sport schlägst.","reaction":"","portrait":"Macher um deine Geschichte als Arzt und創"}`, "portrait was cut off at the 40 letters"},
		{`{"reason":"du die Brücke zwischen Medizin und Sport schlägst.","reaction":"","portrait":"Arzt und創"}`, "portrait has a character outside the Latin alphabet"},
		{`{"reason":"du 創 töpferst.","reaction":"","portrait":"Töpferin"}`, "reason has a character outside the Latin alphabet"},
		{`{"reason":"du töpferst.","reaction":"Toll。","portrait":"Töpferin"}`, "reaction has a character outside the Latin alphabet"},
		{`{"reason":"du töpferst，","reaction":"","portrait":"Töpferin"}`, "reason has a character"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"Гончар"}`, "portrait has a character"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"도예가"}`, "portrait has a character"},
		{`{"reason":"du töpferst.","reaction":"","portrait":"ポッター"}`, "portrait has a character"},
		{`{"reason":"du seit ٢٠١٩ töpferst.","reaction":"","portrait":"Töpferin"}`, "reason has a character"},
		// What German, names and emojis need stays.
		{`{"reason":"du seit 2019 in Köln töpferst! 😄","reaction":"Großartig, oder? 🫠","portrait":"Café-Inhaberin"}`, ""},
		{`{"reason":"du Crème brûlée à la Łódź machst.","reaction":"«Toll» – wirklich!","portrait":"Arzt und Gründer"}`, ""},
		{`{"reason":"du „echte“ Dinge mit 5 µm baust… für 20 € bei 30 °C!","reaction":"Nguyễn, Øre, ẞ, O’Neill, Oʼ 🇩🇪 👍🏽","portrait":"Ingenieurin"}`, ""},
		// A portrait is one or two words, and three for "Arzt und Gründer".
		{`{"reason":"du töpferst.","reaction":"","portrait":"Macher um deine Geschichte"}`, "portrait has 4 words, at most 3 fit"},
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
	var asks atomic.Int32
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		asks.Add(1)
		return language.Answer{Text: "Liebe Mara, du bist toll!", Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	if asks.Load() != 2 {
		t.Errorf("the model was asked %d times, twice at most", asks.Load())
	}
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

// An answer that does not fit is asked for once more, so a slip of the
// model, like a letter of another script in Tim's report of 26 November
// 2026, never reaches the draft and still leaves him one to send.
func TestAnAnswerThatDoesNotFitIsAskedAgain(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	var asks atomic.Int32
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		if asks.Add(1) == 1 {
			return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin und創"}`, Finish: "stop"}, nil
		}
		return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	j := r.d.Jobs()[0]
	want := "Hallo Mara, du bist mir aufgefallen, weil du töpferst.\n\nDeine Geschichte als Töpferin?"
	if asks.Load() != 2 || j.State != "done" || j.Text != want || j.Problem != "" {
		t.Errorf("asked %d times, the job: %+v", asks.Load(), j)
	}
	r.st.Read(func(d *store.Data) {
		if p := d.Person(id); p.Draft != want {
			t.Errorf("kept %q", p.Draft)
		}
	})
}
