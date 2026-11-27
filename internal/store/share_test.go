package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// What Report a Problem takes along of People holds what the person
// screen shows and the profile text, and never the name, the profile
// link, the handle, the notes or the first message, not even inside a
// headline, a passage or the text.
func TestSharedPeopleCarryNoNameOrLink(t *testing.T) {
	st, err := Open(t.TempDir(), func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	var p *Person
	if err := st.Update(func(d *Data) error {
		return carry(d, &p)
	}); err != nil {
		t.Fatal(err)
	}
	shared := st.Share()
	check(t, p, shared)
}

func carry(d *Data, out **Person) error {
	s, err := d.AddSearch("Yogalehrerin Köln", 1, t0)
	if err != nil {
		return err
	}
	p, _ := d.AddFound(Found{
		URL: "https://www.linkedin.com/in/wiebke-quandtmann-7a1", Handle: "wiebke-quandtmann-7a1", Name: "Wiebke Quandtmann",
		Headline: "Inhaberin Yoga Quandtmann | Yogalehrerin", City: "Köln", BornAbout: 1997,
		AgeReason: "Abitur 2016 at Gymnasium Sülz", Roles: "Inhaberin, Yoga QUANDTMANN",
		LastActive: t0.Add(-48 * time.Hour), Text: "Ich bin Wiebke und unterrichte Yoga in Köln.\n- [2026-11-09 · Beitrag: Neue Kurse](https://www.linkedin.com/posts/wq_kurse-123)",
	}, s, 1, t0)
	p.Notes, p.Draft = "met at a fair", "Hallo Wiebke"
	p.Signals = []Signal{{Key: "owner", Label: "Runs Yoga Quandtmann", For: true, Passage: "wiebke-quandtmann-7a1 owns the studio", FoundIn: "roles"}}
	p.Score = 72
	p.Connect(true, t0)
	*out = p
	return nil
}

func check(t *testing.T, p *Person, shared []Shared) {
	t.Helper()
	if len(shared) != 1 {
		t.Fatalf("%d people shared, want 1", len(shared))
	}
	got := shared[0]
	if got.ID != p.ID || got.City != "Köln" || got.BornAbout != 1997 || got.Score != 72 || !got.Connected || got.Review != "confirmed" ||
		got.Text != "Ich bin [name] und unterrichte Yoga in Köln.\n- [2026-11-09 · Beitrag: Neue Kurse]([link])" || !got.HasDraft || got.LastActive == nil || len(got.FoundBy) != 1 || got.FoundBy[0].Search != "Yogalehrerin Köln" {
		t.Errorf("shared = %+v", got)
	}
	if got.Headline != "Inhaberin Yoga [name] | Yogalehrerin" || got.Roles != "Inhaberin, Yoga [name]" {
		t.Errorf("headline %q, roles %q", got.Headline, got.Roles)
	}
	if got.Signals[0].Label != "Runs Yoga [name]" || got.Signals[0].Passage != "[name] owns the studio" {
		t.Errorf("signal %+v", got.Signals[0])
	}
	// The fit goes along as People shows it, its parts adding up to it.
	sum := 0
	for _, pt := range got.FitParts {
		sum += pt.Points
	}
	if len(got.FitParts) != 4 || got.Fit != sum || got.Fit <= 0 || !got.Wrote || got.LastWrote == nil || got.LastWrote.Format("2006-01-02") != "2026-11-09" {
		t.Errorf("fit %d, parts %+v, wrote %v %v", got.Fit, got.FitParts, got.Wrote, got.LastWrote)
	}
	body, err := json.Marshal(shared)
	if err != nil {
		t.Fatal(err)
	}
	for _, never := range []string{"Wiebke", "wiebke", "Quandtmann", "QUANDTMANN", "linkedin", "met at a fair", "Hallo"} {
		if strings.Contains(string(body), never) {
			t.Errorf("%q went along: %s", never, body)
		}
	}
}

// A name is taken out only where it stands as a word of its own, so the
// words around it keep their letters.
func TestUnnamedTakesWholeWordsOnly(t *testing.T) {
	clean := unnamed("Lea Probe", "lea-probe")
	for in, want := range map[string]string{
		"Lea Probe Coaching":           "[name] [name] Coaching",
		"Leadership and probing":       "Leadership and probing",
		"Studio LEA, Köln":             "Studio [name], Köln",
		"see lea-probe for more":       "see [name] for more",
		"Abitur 2016":                  "Abitur 2016",
		"Lea's Yoga":                   "[name]'s Yoga",
		"Bei Lea:Probe gibt es Kaffee": "Bei [name]:[name] gibt es Kaffee",
		"[Post](https://x.example/p?a=1) and www.studio.example.": "[Post]([link]) and [link]",
	} {
		if got := clean(in); got != want {
			t.Errorf("%q gave %q, want %q", in, got, want)
		}
	}
	clean = unnamed("Anna von der Heide", "")
	for in, want := range map[string]string{
		"Annas Studio von der Leyen-Str.": "[name]s Studio von der Leyen-Str.",
		"Heides Blumen, Heide's Café":     "[name]s Blumen, [name]'s Café",
	} {
		if got := clean(in); got != want {
			t.Errorf("%q gave %q, want %q", in, got, want)
		}
	}
	if got := unnamed("Jörg Müller", "")("Joerg Mueller und Jörg Müller"); got != "[name] [name] und [name] [name]" {
		t.Errorf("umlauts written out gave %q", got)
	}
	if got := unnamed("Al", "")("Al runs it"); got != "Al runs it" {
		t.Errorf("a name of two letters took %q", got)
	}
}

// What the window writes into the app's log loses the name and handle of
// every person, whoever they are, also when a tooltip named the person a
// run reads, issue #104. Words that only start like a name keep their
// letters, and a person found later is taken out by the next call.
func TestUnnamedTakesEveryPersonOutOfTheLog(t *testing.T) {
	st, err := Open(t.TempDir(), func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, handle string) {
		t.Helper()
		if err := st.Update(func(d *Data) error {
			s, err := d.AddSearch("Coach Bonn", 1, t0)
			if err != nil && s == nil {
				return err
			}
			d.AddFound(Found{URL: "https://www.linkedin.com/in/" + handle, Handle: handle, Name: name, City: "Bonn"}, s, 1, t0)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := st.Unnamed()("Reading the profile of Lea Probe"); got != "Reading the profile of Lea Probe" {
		t.Errorf("with nobody known it gave %q", got)
	}
	add("Lea Probe", "lea-probe-7a1")
	add("Jörg Müller-Lüdenscheidt", "joerg-ml")
	// Exa gives a name as it finds it, in lower case too, and some are
	// short. The review of #129 found both left in the log.
	add("ida von der beispiel", "idvdb")
	add("Bo Li", "bl")
	add("Al Probst", "ap")
	clean := st.Unnamed()
	for in, want := range map[string]string{
		`pressed button.btn "Reading the profile of Lea Probe. 1 of 3 profiles"`: `pressed button.btn "Reading the profile of [name] [name]. 1 of 3 profiles"`,
		"Leas Studio, LEA PROBE":                      "[name]s Studio, [name] [name]",
		"Leadership and probing":                      "Leadership and probing",
		"GET /api/people/12 for lea-probe-7a1 done":   "GET /api/people/12 for [name] done",
		"https://www.linkedin.com/in/joerg-ml/":       "https://www.linkedin.com/in/[name]/",
		"Joerg Mueller-Luedenscheidt und Jörg":        "[name] [name]-[name] und [name]",
		"Run done. 2 new people":                      "Run done. 2 new people",
		"Reading the profile of ida von der beispiel": "Reading the profile of [name] von der [name]",
		"Reading the profile of Bo Li and 1 more":     "Reading the profile of [name] [name] and 1 more",
		"Al and als, Bos":                             "[name] and als, Bos",
		"":                                            "",
	} {
		if got := clean(in); got != want {
			t.Errorf("%q gave %q, want %q", in, got, want)
		}
	}
	add("Wiebke Quandtmann", "wq")
	if got := clean("Wiebke"); got != "Wiebke" {
		t.Errorf("an earlier call took out a person found after it: %q", got)
	}
	if got := st.Unnamed()("Wiebke wq"); got != "[name] wq" {
		t.Errorf("a later call gave %q, want the new name out and a handle of two letters kept", got)
	}
}
