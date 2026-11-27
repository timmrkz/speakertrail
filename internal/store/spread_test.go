package store

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// spreadData is a file with searches like Tim's, kinds of people asked in
// a few cities, and invented people some of them found.
func spreadData(t *testing.T) *Data {
	t.Helper()
	d := &Data{}
	d.fill()
	d.Exclusions = []*Exclusion{}
	n := 0
	search := func(query string, people, connected int) *Search {
		s, err := d.AddSearch(query, 0, t0)
		if err != nil {
			t.Fatal(err)
		}
		s.Runs, s.LastRun = 1, t0
		for i := range people {
			n++
			p, _ := d.AddFound(Found{URL: fmt.Sprintf("https://www.linkedin.com/in/probe-%d", n), Name: fmt.Sprintf("Probe %d", n), City: "Köln"}, s, 0, t0)
			p.Connected = i < connected
		}
		return s
	}
	search("Gründer unter 30 Köln", 10, 3)
	search("Gründer unter 30 NRW", 10, 2)
	search("Yogalehrerin Köln", 4, 1)
	search("Yogalehrerin Bonn", 4, 0)
	// No connection yet, so not spread.
	search("Tanzschule Inhaberin Köln", 8, 0)
	// No place, so not spread.
	search("Physiotherapie eigene Praxis", 5, 2)
	return d
}

func queries(s []*Search) []string {
	var out []string
	for _, x := range s {
		out = append(out, x.Query)
	}
	return out
}

func TestSpreadTakesTurnsNearestFirst(t *testing.T) {
	d := spreadData(t)
	if err := d.SetSetting(HomeCity, json.RawMessage(`"Köln"`), t0); err != nil {
		t.Fatal(err)
	}
	made := d.Spread(4, t0)
	// The kind with the most connections first, then the next, taking
	// turns, each its nearest city it was not asked in.
	want := []string{"Gründer unter 30 Leverkusen", "Yogalehrerin Leverkusen", "Gründer unter 30 Bergisch Gladbach", "Yogalehrerin Bergisch Gladbach"}
	if got := queries(made); !slices.Equal(got, want) {
		t.Fatalf("made %q, want %q", got, want)
	}
	for _, s := range made {
		if s.Status != "candidate" || !s.NextRun.IsZero() || d.Search(s.ID) != s {
			t.Errorf("%s is not a new search due at once: %+v", s.Query, s)
		}
	}
	if made[0].From != "Gründer unter 30 Köln" || made[1].From != "Yogalehrerin Köln" {
		t.Errorf("made from %q and %q", made[0].From, made[1].From)
	}
	// The next run goes on where this one stopped, past Bonn, where
	// Yogalehrerin was asked already.
	if got := queries(d.Spread(2, t0)); !slices.Equal(got, []string{"Gründer unter 30 Bonn", "Yogalehrerin Solingen"}) {
		t.Errorf("then %q", got)
	}
}

func TestSpreadWithoutWhereYouAreTakesTheBiggestCities(t *testing.T) {
	d := spreadData(t)
	if got := queries(d.Spread(3, t0)); !slices.Equal(got, []string{"Gründer unter 30 Bonn", "Yogalehrerin Düsseldorf", "Gründer unter 30 Düsseldorf"}) {
		t.Errorf("made %q", got)
	}
}

func TestSpreadRunsOut(t *testing.T) {
	d := spreadData(t)
	// Every big city for both kinds, less those asked already: 29 for
	// Gründer unter 30, asked in Köln and NRW, which is no city, and 28
	// for Yogalehrerin, asked in Köln and Bonn.
	made := d.Spread(1000, t0)
	if len(made) != 57 {
		t.Errorf("made %d searches, want 57", len(made))
	}
	if again := d.Spread(5, t0); len(again) != 0 {
		t.Errorf("made %q once every city was asked", queries(again))
	}
	if d.Spread(0, t0) != nil || d.Spread(-1, t0) != nil {
		t.Error("made searches without room for them")
	}
}

func TestSpreadLeavesKindsTimDoesNotWant(t *testing.T) {
	d := spreadData(t)
	// Mostly paused: Yogalehrerin Köln and Bonn are paused, so it is
	// left alone.
	for _, s := range d.Searches {
		if s.Query == "Yogalehrerin Köln" || s.Query == "Yogalehrerin Bonn" {
			s.Pause(t0)
		}
	}
	// An exclusion: a kind with its words is never asked, in any case, as
	// the exclusions Tim writes have capitals.
	if _, err := d.AddExclusion("Unter 30", t0); err != nil {
		t.Fatal(err)
	}
	if made := d.Spread(5, t0); len(made) != 0 {
		t.Errorf("made %q", queries(made))
	}
	// Resuming one search makes the kind half paused, which is not mostly.
	for _, s := range d.Searches {
		if s.Query == "Yogalehrerin Bonn" {
			s.Resume(t0)
		}
	}
	if got := queries(d.Spread(1, t0)); !slices.Equal(got, []string{"Yogalehrerin Düsseldorf"}) {
		t.Errorf("made %q", got)
	}
}

// Pausing the searches Tim added stops a kind, however many searches a
// run made of it, which never speak for it.
func TestSpreadStopsWhenTimPausesWhatHeAdded(t *testing.T) {
	d := spreadData(t)
	if len(d.Spread(10, t0)) != 10 {
		t.Fatal("made too few")
	}
	for _, s := range d.Searches {
		if s.Query == "Yogalehrerin Köln" || s.Query == "Yogalehrerin Bonn" {
			s.Pause(t0)
		}
	}
	for _, s := range d.Spread(10, t0) {
		if strings.HasPrefix(s.Query, "Yogalehrerin") {
			t.Errorf("made %s after Tim paused the searches he added", s.Query)
		}
	}
}
