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
	var d Data
	d.fill()
	s, err := d.AddSearch("Yogalehrerin Köln", 1, t0)
	if err != nil {
		t.Fatal(err)
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
	d.AddExclusion("Online Marketing", t0)

	shared := d.Share()
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
		"Leadership and probes":        "Leadership and probes",
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
	if got := unnamed("Al", "")("Al runs it"); got != "Al runs it" {
		t.Errorf("a name of two letters took %q", got)
	}
}
