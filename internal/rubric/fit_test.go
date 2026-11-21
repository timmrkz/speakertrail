package rubric

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func part(t *testing.T, parts []Part, key string) Part {
	t.Helper()
	for _, p := range parts {
		if p.Key == key {
			return p
		}
	}
	t.Fatalf("no part %s in %v", key, parts)
	return Part{}
}

// Tim, 21 November 2026: a person who posted 5 days ago comes before one
// who posted 5 months ago, and a person near him before one far away.
func TestFitActiveLately(t *testing.T) {
	now := day("2026-11-21")
	for _, tc := range []struct {
		name          string
		active, wrote time.Time
		known         bool
		points        int
		why           string
	}{
		{"5 days ago", day("2026-11-16"), day("2026-11-16"), true, 22, "Wrote on 16 November 2026"},
		{"30 days ago", day("2026-10-22"), day("2026-10-22"), true, 13, "Wrote on 22 October 2026"},
		{"5 months ago", day("2026-06-24"), day("2026-06-24"), true, 1, "Wrote on 24 June 2026"},
		{"kinds not known", day("2026-11-16"), time.Time{}, false, 22, "Last active on 16 November 2026"},
		{"only a like", day("2026-11-16"), time.Time{}, true, 11, "Reacted on 16 November 2026, wrote nothing"},
		{"a like after a comment", day("2026-11-20"), day("2026-11-14"), true, 21, "Wrote on 14 November 2026, reacted since"},
		{"a like after an old comment", day("2026-11-20"), day("2026-07-01"), true, 12, "Reacted on 20 November 2026, wrote last on 1 July 2026"},
		{"nothing", time.Time{}, time.Time{}, false, 0, "No activity on LinkedIn known"},
	} {
		_, parts := Fit(FitPerson{LastActive: tc.active, LastWrote: tc.wrote, Wrote: tc.known, Now: now})
		got := part(t, parts, "active")
		if got.Points != tc.points || got.Why != tc.why {
			t.Errorf("%s: %d %q, want %d %q", tc.name, got.Points, got.Why, tc.points, tc.why)
		}
	}
	_, recent := Fit(FitPerson{LastActive: day("2026-11-16"), Now: now})
	_, older := Fit(FitPerson{LastActive: day("2026-06-24"), Now: now})
	if part(t, recent, "active").Points <= part(t, older, "active").Points {
		t.Error("5 days ago does not count for more than 5 months ago")
	}
}

func TestFitNearYou(t *testing.T) {
	for _, tc := range []struct {
		name   string
		p      FitPerson
		points int
		why    string
	}{
		{"same city", FitPerson{Home: "Köln", City: "Köln", Near: true}, 25, "Köln, where you are"},
		{"25 km", FitPerson{Home: "Köln", City: "Bonn", Km: 25, Near: true}, 16, "Bonn, about 25 km"},
		{"125 km", FitPerson{Home: "Köln", City: "Münster", Km: 125, Near: true}, 3, "Münster, about 125 km"},
		{"no city", FitPerson{Home: "Köln"}, 13, "City not known"},
		{"a city without a centre", FitPerson{Home: "Köln", City: "Hückeswagen"}, 13, "Hückeswagen, distance not known"},
		{"not set", FitPerson{City: "Bonn"}, 13, "Where you are is not set in Settings"},
	} {
		_, parts := Fit(tc.p)
		got := part(t, parts, "near")
		if got.Points != tc.points || got.Why != tc.why {
			t.Errorf("%s: %d %q, want %d %q", tc.name, got.Points, got.Why, tc.points, tc.why)
		}
	}
}

func TestFitYounger(t *testing.T) {
	now := day("2026-11-21")
	for _, tc := range []struct {
		born   int
		points int
	}{
		{2002, 25}, {1996, 25}, {1991, 13}, {1987, 3}, {1986, 0}, {1970, 0}, {0, 13},
	} {
		_, parts := Fit(FitPerson{BornAbout: tc.born, Now: now})
		if got := part(t, parts, "younger"); got.Points != tc.points {
			t.Errorf("born %d: %d points, want %d", tc.born, got.Points, tc.points)
		}
	}
	_, parts := Fit(FitPerson{BornAbout: 2000, AgeReason: "Bachelor from 2019", Now: now})
	if got := part(t, parts, "younger").Why; got != "Bachelor from 2019, so about 26" {
		t.Errorf("why %q", got)
	}
}

func TestFitWhoTheyAre(t *testing.T) {
	for _, tc := range []struct {
		keys   []string
		points int
		why    string
	}{
		{nil, 0, "Nothing beyond running their own business"},
		// Running their own business, age and activity count elsewhere.
		{[]string{"owner_operator", "young", "active_on_linkedin"}, 0, "Nothing beyond running their own business"},
		{[]string{"works_with_people", "athlete"}, 17, "Works with people, Athlete"},
		{[]string{"works_with_people", "athlete", "author", "runs_events"}, 25, "Works with people, Runs their own events, Author, Athlete"},
		{[]string{"works_with_people", "funded"}, 0, "Works with people, against Raised money"},
		{[]string{"funded"}, 0, "Against Raised money"},
	} {
		var sigs []Signal
		for _, d := range Defs {
			for _, k := range tc.keys {
				if d.Key == k {
					sigs = append(sigs, Signal{Key: k, For: d.For})
				}
			}
		}
		_, parts := Fit(FitPerson{Signals: sigs})
		got := part(t, parts, "who")
		if got.Points != tc.points || got.Why != tc.why {
			t.Errorf("%v: %d %q, want %d %q", tc.keys, got.Points, got.Why, tc.points, tc.why)
		}
	}
}

// The worked example in docs/fit-score.md, without the part that learns:
// a coach in Bonn, about 26, who commented 4 days ago.
func TestFitAddsUp(t *testing.T) {
	now := day("2026-11-21")
	fit, parts := Fit(FitPerson{
		Signals:   []Signal{{Key: "works_with_people", For: true}, {Key: "athlete", For: true}},
		BornAbout: 2000, AgeReason: "Bachelor from 2019",
		LastActive: day("2026-11-17"), LastWrote: day("2026-11-17"), Wrote: true,
		Home: "Köln", City: "Bonn", Km: 25, Near: true, Now: now,
	})
	sum := 0
	for _, p := range parts {
		sum += p.Points
	}
	if fit != sum || fit != 17+25+23+16 {
		t.Errorf("fit %d, parts %v", fit, parts)
	}
}

func TestWriting(t *testing.T) {
	text := "# Lea Probe\n\n## Activity\n\n" +
		"- [2026-11-02 · Gefällt mir: Ein Beitrag](https://www.linkedin.com/posts/x)\n" +
		"- [2026-10-30 · Kommentar: Starker Kurs!](https://www.linkedin.com/posts/y)\n" +
		"- [2026-11-05 · Likes: A post](https://www.linkedin.com/posts/z)\n" +
		"- [2026-09-01 · Mein erstes Jahr als Trainerin](https://www.linkedin.com/posts/w)\n"
	got, known := Writing(text)
	if !known || !got.Equal(day("2026-10-30")) {
		t.Errorf("Writing = %v, %v, want 30 October", got, known)
	}
	if got, known := Writing("- [2026-11-02 · Gefällt mir: Ein Beitrag](x)"); !known || !got.IsZero() {
		t.Errorf("only a like: %v, %v", got, known)
	}
	if _, known := Writing("# Lea Probe\n\nPersonal Trainerin"); known {
		t.Error("a profile without activity says what kind its activity is")
	}
}
