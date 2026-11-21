package store

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// first always picks the first variation.
func first(int) int { return 0 }

func tmpl(t *testing.T, raw string) Template {
	t.Helper()
	var out Template
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTimsTemplateMakesHisMessage(t *testing.T) {
	got := DefaultTemplate().Expand(map[string]string{
		FirstNameField: "Ada", PortraitField: "Inhaberin",
		ReasonField: "du ein Studio führst.", ReactionField: " Das finde ich toll!",
	}, first)
	want := "Hallo Ada,\n\nsuper cool mit dir zu connecten. Du bist mir in meinem LinkedIn Feed aufgefallen, weil du ein Studio führst. Das finde ich toll!\n\n" +
		"Ich würde dich total gern in meinen Podcast \"My First Memory\" einladen, um deine Geschichte als Inhaberin zu erzählen. Die erste Frage ist immer: was ist deine erste Erinnerung?\n\n" +
		"Wenn du Lust hast, dann lass uns die Tage gern einen kurzen Video Call machen, um uns ein bisschen kennen zu lernen.\n\nLiebe Grüße, Tim"
	if got != want {
		t.Errorf("Tim's template with the first of each part made\n%s\nwant\n%s", got, want)
	}
}

func TestEachPickTakesItsVariation(t *testing.T) {
	tp := tmpl(t, `{"origin":["#a# #b# #reason#"],"a":["x","y","z"],"b":["#c#",""],"c":["deep"]}`)
	if err := tp.Check(); err != nil {
		t.Fatal(err)
	}
	picks := []int{0, 2, 0, 0}
	n := []int{}
	got := tp.Expand(map[string]string{ReasonField: "R"}, func(k int) int {
		n = append(n, k)
		p := picks[0]
		picks = picks[1:]
		return p
	})
	if got != "z deep R" {
		t.Errorf("got %q, want %q", got, "z deep R")
	}
	if want := []int{1, 3, 2, 1}; !slices.Equal(n, want) {
		t.Errorf("asked to pick from %v, want %v", n, want)
	}
}

func TestWhatTheModelWroteIsNeverReadAsAPart(t *testing.T) {
	tp := tmpl(t, `{"origin":["#reason#"],"secret":["no"]}`)
	if got := tp.Expand(map[string]string{ReasonField: "#secret# <b>x</b>"}, first); got != "#secret# <b>x</b>" {
		t.Errorf("got %q, the reason should stay as it was written", got)
	}
}

func TestATemplateThatCannotWorkIsRefused(t *testing.T) {
	cases := map[string]struct{ raw, says string }{
		"no origin":          {`{"start":["#reason#"]}`, "origin"},
		"no reason":          {`{"origin":["Hallo"]}`, "#reason#"},
		"reason not reached": {`{"origin":["Hallo"],"hook":["#reason#"]}`, "#reason#"},
		"unknown part":       {`{"origin":["#reason# #nope#"]}`, "#nope#"},
		"a field's name":     {`{"origin":["#reason#"],"first_name":["x"]}`, "filled in by the app"},
		"a model's field":    {`{"origin":["#reason#"],"reason":["x"]}`, "language model"},
		"bad name":           {`{"origin":["#reason#"],"Opener":["x"]}`, "cannot name a part"},
		"itself":             {`{"origin":["#reason# #a#"],"a":["#b#"],"b":["#a#"]}`, "holds itself"},
		"control":            {`{"origin":["#reason#\u0000"]}`, "invisible character, U+0000, in line 1 after \"#reason#\""},
		"zero width space":   {`{"origin":["#reason#"],"a":["Hallo\n\u200bdu"]}`, "U+200B, in line 2"},
		"right to left":      {`{"origin":["#reason#"],"a":["\u202eHallo"]}`, "U+202E"},
		"byte order mark":    {`{"origin":["#reason#"],"a":["\ufeffHallo"]}`, "U+FEFF"},
		"line separator":     {`{"origin":["#reason#"],"a":["Hallo\u2028du"]}`, "U+2028"},
		"too long":           {`{"origin":["#reason#` + strings.Repeat("x", 2001) + `"]}`, "at most 2000"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := tmpl(t, c.raw).Check()
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Errorf("Check said %v, want it to say %q", err, c.says)
			}
		})
	}
	twice := Template{Parts: []Part{{"origin", []string{"#reason#"}}, {"a", []string{"x"}}, {"a", []string{"y"}}}}
	if err := twice.Check(); err == nil || !strings.Contains(err.Error(), "two parts") {
		t.Errorf("two parts of one name: %v", err)
	}
	if err := tmpl(t, `{"origin":["#reason#"],"a":["`+strings.Repeat("x", 2000)+`"]}`).Check(); err != nil {
		t.Errorf("2000 letters should fit: %v", err)
	}
}

// Tim had nothing more to say in extra for now, so a part may have no
// variation at all, and is then left out of every message.
func TestAPartWithoutVariationsIsLeftOut(t *testing.T) {
	tp := tmpl(t, `{"origin":["Hallo#extra# #reason#"],"extra":[]}`)
	if err := tp.Check(); err != nil {
		t.Fatalf("a part without variations was refused: %v", err)
	}
	if got := tp.Expand(map[string]string{ReasonField: "R"}, first); got != "Hallo R" {
		t.Errorf("got %q, want %q", got, "Hallo R")
	}
	d := &Data{}
	if err := d.SetTemplatePart("extra", []string{}, -1); err != nil {
		t.Fatalf("taking out every variation of extra: %v", err)
	}
	b, _ := json.Marshal(d.FirstMessageTemplate())
	if !strings.Contains(string(b), `"extra":[]`) {
		t.Errorf("extra is kept as %s", b)
	}
	// What is not hidden stays: the joiner inside an emoji, umlauts, and
	// a tab nowhere but the prompt.
	if err := (Template{Parts: []Part{{"origin", []string{"#reason# 👨\u200d👩\u200d👧 Grüße"}}}}).Check(); err != nil {
		t.Errorf("an emoji with joiners was refused: %v", err)
	}
}

func TestThePromptHoldsNothingHidden(t *testing.T) {
	d := &Data{Settings: map[string]SettingValue{}}
	set := func(prompt string) error {
		b, _ := json.Marshal(prompt)
		return d.SetSetting(FirstMessagePrompt, b, time.Now())
	}
	if err := set("Schreib\tkurz\n\n{{profile}} 😄"); err != nil {
		t.Errorf("a prompt with a tab and an emoji: %v", err)
	}
	err := set("Schreib kurz\nTim\u200b mag Sport\n{{profile}}")
	if err == nil || !strings.Contains(err.Error(), `U+200B, in line 2 after "Tim"`) {
		t.Errorf("a zero width space in the prompt: %v", err)
	}
	if err := set(DefaultFirstMessagePrompt); err != nil {
		t.Errorf("Tim's prompt: %v", err)
	}
}

func TestPartsDeeperThanTwentyAreRefused(t *testing.T) {
	parts := []Part{{"origin", []string{"#reason# #p1#"}}}
	for i := 1; i <= 21; i++ {
		v := "end"
		if i < 21 {
			v = "#p" + itoa(i+1) + "#"
		}
		parts = append(parts, Part{"p" + itoa(i), []string{v}})
	}
	if err := (Template{Parts: parts}).Check(); err == nil || !strings.Contains(err.Error(), "deep") {
		t.Errorf("22 levels: %v", err)
	}
	if err := (Template{Parts: parts[:19]}).Check(); err == nil {
		t.Error("a part that uses a missing one should be refused")
	}
	short := append(parts[:19:19], Part{"p19", []string{"end"}})
	short[18] = Part{"p18", []string{"#p19#"}}
	if err := (Template{Parts: short}).Check(); err != nil {
		t.Errorf("19 levels should do: %v", err)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestTheTemplateKeepsItsOrderAndLeavesNotesOut(t *testing.T) {
	tp := tmpl(t, `{"_about":["a note"],"zeta":["z"],"origin":["#zeta# #reason#"],"alpha":["a"]}`)
	b, err := json.Marshal(tp)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"zeta":["z"],"origin":["#zeta# #reason#"],"alpha":["a"]}`; string(b) != want {
		t.Errorf("written as %s, want %s", b, want)
	}
}

func TestTimEditsThePartsOfHisTemplate(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	update := func(fn func(d *Data) error) error { return s.Update(fn) }
	if err := update(func(d *Data) error {
		return d.SetTemplatePart("opener", []string{"schön dich zu sehen", ""}, -1)
	}); err != nil {
		t.Fatal(err)
	}
	// A new part goes where it is asked to, and origin uses it.
	if err := update(func(d *Data) error {
		if err := d.SetTemplatePart("wave", []string{"👋"}, 1); err != nil {
			return err
		}
		o, _ := d.FirstMessageTemplate().Part(TemplateOrigin)
		return d.SetTemplatePart(TemplateOrigin, []string{o.Variations[0] + " #wave#"}, -1)
	}); err != nil {
		t.Fatal(err)
	}
	// A part still used stays, and so does origin.
	err := update(func(d *Data) error { return d.RemoveTemplatePart("wave") })
	if err == nil || !strings.Contains(err.Error(), "origin still uses #wave#") {
		t.Errorf("removing a part in use: %v", err)
	}
	if err := update(func(d *Data) error { return d.RemoveTemplatePart(TemplateOrigin) }); err == nil {
		t.Error("origin should stay")
	}
	if err := update(func(d *Data) error { return d.RemoveTemplatePart("nope") }); !errors.Is(err, ErrNoPart) {
		t.Errorf("removing a part that is not there: %v", err)
	}
	// A change that breaks the template is refused and changes nothing.
	if err := update(func(d *Data) error { return d.SetTemplatePart("greeting", []string{"#nope#"}, -1) }); err == nil {
		t.Error("a variation using a missing part should be refused")
	}

	// It is all there after the app opens again, in the same order.
	again := open(t, dir)
	var got Template
	again.Read(func(d *Data) { got = d.FirstMessageTemplate() })
	var names []string
	for _, p := range got.Parts {
		names = append(names, p.Name)
	}
	if strings.Join(names[:3], " ") != "origin wave greeting" {
		t.Errorf("the parts are %v, want wave second", names)
	}
	if op, _ := got.Part("opener"); len(op.Variations) != 2 || op.Variations[1] != "" {
		t.Errorf("opener is %q", op.Variations)
	}
	if gr, _ := got.Part("greeting"); gr.Variations[0] != "Hallo" {
		t.Errorf("the refused change stayed: %q", gr.Variations)
	}
}

func TestTheDefaultTemplateComesBack(t *testing.T) {
	d := &Data{}
	before := d.FirstMessageTemplate()
	if err := d.SetTemplatePart("greeting", []string{"Hi"}, -1); err != nil {
		t.Fatal(err)
	}
	changed := d.FirstMessageTemplate()
	if d.Template == nil {
		t.Fatal("a changed template should be kept")
	}
	// Changing it back by hand keeps nothing, as a setting does.
	if err := d.SetTemplatePart("greeting", []string{"Hallo"}, -1); err != nil {
		t.Fatal(err)
	}
	if d.Template != nil {
		t.Error("the template as it was at first should not be kept")
	}
	if err := d.SetTemplate(changed); err != nil {
		t.Fatal(err)
	}
	d.ResetTemplate()
	after, _ := json.Marshal(d.FirstMessageTemplate())
	want, _ := json.Marshal(before)
	if string(after) != string(want) {
		t.Error("reset should bring Tim's template back")
	}
	// What a caller does with the template it was given never reaches the
	// one kept.
	got := d.FirstMessageTemplate()
	got.Parts[0].Variations[0] = "changed"
	if d.FirstMessageTemplate().Parts[0].Variations[0] == "changed" {
		t.Error("the template handed out should be a copy")
	}
}

// Parts that each use the next four times stay inside every other limit
// but would make a message of millions of letters, and walking every
// path took 100 s inside Store.Update. Each part is walked once, and the
// template is refused for the message it would make.
func TestATemplateThatWouldMakeAMessageWithoutEndIsRefused(t *testing.T) {
	parts := []Part{{"origin", []string{"#reason# #p1#"}}}
	for i := 1; i <= 13; i++ {
		v := "end"
		if i < 13 {
			n := "#p" + itoa(i+1) + "#"
			v = n + n + n + n
		}
		parts = append(parts, Part{"p" + itoa(i), []string{v}})
	}
	err := (Template{Parts: parts}).Check()
	if err == nil || !strings.Contains(err.Error(), "more than 3000 letters") {
		t.Errorf("a message of 4^12 parts: %v", err)
	}
	// One part used twice in two places is fine, and counted once each.
	ok := Template{Parts: []Part{{"origin", []string{"#reason# #a# #b#"}}, {"a", []string{"#c#"}}, {"b", []string{"#c#"}}, {"c", []string{strings.Repeat("x", 1300)}}}}
	if err := ok.Check(); err != nil {
		t.Errorf("2,600 letters and the reason should fit: %v", err)
	}
	ok.Parts[3].Variations = append(ok.Parts[3].Variations, strings.Repeat("y", 1450))
	if err := ok.Check(); err == nil {
		t.Error("3,000 letters with the reason should not fit")
	}
}

// A prompt kept from the builds of pull request 38, which asked for a
// whole message, goes when the file is read.
func TestTheOldPromptGoes(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	s.Update(func(d *Data) error {
		d.Settings["first_message_prompt"] = SettingValue{Value: json.RawMessage(`"Write a whole message {{profile}}"`)}
		return nil
	})
	open(t, dir).Read(func(d *Data) {
		if _, ok := d.Settings["first_message_prompt"]; ok {
			t.Error("the old prompt is still there")
		}
	})
}
