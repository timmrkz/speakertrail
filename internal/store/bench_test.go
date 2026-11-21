package store

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// What a change costs with Tim's people, each with a profile text.
func BenchmarkUpdateWithProfileTexts(b *testing.B) {
	s, err := Open(b.TempDir(), func() time.Time { return t0 })
	if err != nil {
		b.Fatal(err)
	}
	text := strings.Repeat("Ich töpfere in Köln. ", 380)[:8000]
	if err := s.Update(func(d *Data) error {
		search, _ := d.AddSearch("Keramik Köln", 0, t0)
		for i := range 1000 {
			d.AddFound(Found{URL: "https://www.linkedin.com/in/p" + strconv.Itoa(i), Name: "Eva Beispiel", Text: text}, search, 1, t0)
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if err := s.Update(func(d *Data) error { d.People[0].Notes += "x"; return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

// What a change costs that brings a new text, as a check that finds
// someone does.
func BenchmarkUpdateWithANewText(b *testing.B) {
	s, err := Open(b.TempDir(), func() time.Time { return t0 })
	if err != nil {
		b.Fatal(err)
	}
	text := strings.Repeat("Ich töpfere in Köln. ", 380)[:8000]
	if err := s.Update(func(d *Data) error {
		search, _ := d.AddSearch("Keramik Köln", 0, t0)
		for i := range 1000 {
			d.AddFound(Found{URL: "https://www.linkedin.com/in/p" + strconv.Itoa(i), Name: "Eva Beispiel", Text: text}, search, 1, t0)
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	n := 0
	b.ResetTimer()
	for b.Loop() {
		n++
		if err := s.Update(func(d *Data) error { d.SetProfileText(d.People[0], text+strconv.Itoa(n), t0); return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListPeople lists 5000 people the way People asks, the best fit
// first, which works out every person's fit on each call.
func BenchmarkListPeople(b *testing.B) {
	d := &Data{}
	d.fill()
	d.Exclusions = []*Exclusion{}
	s, _ := d.AddSearch("Yogalehrerin NRW", 0, t0)
	cities := []string{"Köln", "Bonn", "Essen", "Münster", ""}
	for i := range 5000 {
		p, _ := d.AddFound(Found{URL: fmt.Sprintf("https://www.linkedin.com/in/p%d", i), Name: fmt.Sprintf("Person %d", i), Headline: "Yogalehrerin",
			City: cities[i%len(cities)], BornAbout: 1980 + i%25, LastActive: t0.Add(-time.Duration(i%180) * 24 * time.Hour)}, s, 0, t0)
		p.Signals = []Signal{{Key: "works_with_people", For: true}}
	}
	_ = d.SetSetting(HomeCity, []byte(`"Köln"`), t0)
	b.ResetTimer()
	for range b.N {
		if _, _, err := d.ListPeople(PeopleQuery{Sort: "", Now: t0}); err != nil {
			b.Fatal(err)
		}
	}
}
