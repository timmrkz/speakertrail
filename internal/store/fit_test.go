package store

import (
	"encoding/json"
	"testing"
	"time"
)

// Tim, 21 November 2026: who wrote 5 days ago comes before who wrote 5
// months ago, and who is near before who is far, so he picks from the top.
func TestListPeopleByFit(t *testing.T) {
	d := &Data{}
	d.fill()
	d.Exclusions = []*Exclusion{}
	s, _ := d.AddSearch("Yogalehrerin NRW", 0, t0)
	add := func(name, city string, active time.Time) *Person {
		p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/" + name, Name: name, Headline: "Yogalehrerin", City: city, BornAbout: 1996, LastActive: active}, s, 0, t0)
		return p
	}
	lately, longAgo := t0.Add(-5*24*time.Hour), t0.Add(-150*24*time.Hour)
	far := add("Ida", "Bielefeld", lately)
	quiet := add("Jo", "Bonn", longAgo)
	near := add("Kim", "Bonn", lately)

	list := func() []string {
		out, _, err := d.ListPeople(PeopleQuery{Now: t0})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range out {
			names = append(names, p.Name)
		}
		return names
	}
	// Without Where you are, distance changes no order.
	if got := list(); len(got) != 3 || got[2] != quiet.Name {
		t.Errorf("without Where you are: %v, want %s last", got, quiet.Name)
	}
	if err := d.SetSetting(HomeCity, json.RawMessage(`"Cologne"`), t0); err != nil {
		t.Fatal(err)
	}
	if got := d.Text(HomeCity); got != "Köln" {
		t.Errorf("Where you are is %q, want Köln", got)
	}
	if got := list(); got[0] != near.Name || got[1] != far.Name || got[2] != quiet.Name {
		t.Errorf("from Köln: %v, want %s, %s, %s", got, near.Name, far.Name, quiet.Name)
	}
	fit, parts := d.Fit(near, t0)
	sum := 0
	for _, p := range parts {
		sum += p.Points
	}
	if fit != sum || len(parts) != 4 {
		t.Errorf("fit %d is not its parts %v", fit, parts)
	}
	if parts[3].Why != "Bonn, about 24 km" {
		t.Errorf("near you says %q", parts[3].Why)
	}
}

func TestHomeCity(t *testing.T) {
	d := &Data{}
	d.fill()
	for _, bad := range []string{`"Berlin"`, `"Ruhr Region"`, `12`} {
		if err := d.SetSetting(HomeCity, json.RawMessage(bad), t0); err == nil {
			t.Errorf("Where you are took %s", bad)
		}
	}
	if err := d.SetSetting(HomeCity, json.RawMessage(`" duesseldorf "`), t0); err != nil || d.Text(HomeCity) != "Düsseldorf" {
		t.Errorf("Where you are: %q, %v", d.Text(HomeCity), err)
	}
	if err := d.SetSetting(HomeCity, json.RawMessage(`""`), t0); err != nil || len(d.Settings) != 0 {
		t.Errorf("an empty Where you are should take the change back: %v %v", err, d.Settings)
	}
}

// A reaction counts half as much as writing, read from the profile text
// whenever it is set, and again when the file is read.
func TestProfileTextSaysWhoWrites(t *testing.T) {
	d := &Data{}
	d.fill()
	s, _ := d.AddSearch("Yogalehrerin NRW", 0, t0)
	text := "# Lea Probe\n\n## Activity\n\n- [2026-11-09 · Gefällt mir: Ein Beitrag](https://www.linkedin.com/posts/x)\n- [2026-10-01 · Kommentar: Schön!](https://www.linkedin.com/posts/y)"
	p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/lea-probe", Name: "Lea Probe", LastActive: t0.Add(-24 * time.Hour), Text: text}, s, 0, t0)
	want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !p.LinkedIn.Wrote || !p.LinkedIn.LastWrote.Equal(want) {
		t.Errorf("after a text arrived: %v, %v", p.LinkedIn.LastWrote, p.LinkedIn.Wrote)
	}
	again := &Data{People: []*Person{{LinkedIn: LinkedIn{URL: p.LinkedIn.URL}}}}
	again.attach(d.profileTexts())
	if l := again.People[0].LinkedIn; !l.Wrote || !l.LastWrote.Equal(want) {
		t.Errorf("after the file was read: %v, %v", l.LastWrote, l.Wrote)
	}
}
