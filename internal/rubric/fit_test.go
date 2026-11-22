package rubric

import (
	"strings"
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
		{"5 days ago", day("2026-11-16"), day("2026-11-16"), true, 27, "Wrote on 16 November 2026"},
		{"30 days ago", day("2026-10-22"), day("2026-10-22"), true, 15, "Wrote on 22 October 2026"},
		{"5 months ago", day("2026-06-24"), day("2026-06-24"), true, 1, "Wrote on 24 June 2026"},
		{"kinds not known", day("2026-11-16"), time.Time{}, false, 13, "Last active on 16 November 2026, not known whether they wrote"},
		{"only a like", day("2026-11-16"), time.Time{}, true, 13, "Reacted on 16 November 2026, wrote nothing"},
		{"a like after a comment", day("2026-11-20"), day("2026-11-14"), true, 26, "Wrote on 14 November 2026, reacted since"},
		{"a like after an old comment", day("2026-11-20"), day("2026-07-01"), true, 15, "Reacted on 20 November 2026, wrote last on 1 July 2026"},
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
		{"same city", FitPerson{Home: "Köln", City: "Köln", Near: true}, 20, "Köln, where you are"},
		{"25 km", FitPerson{Home: "Köln", City: "Bonn", Km: 25, Near: true}, 13, "Bonn, about 25 km"},
		{"125 km", FitPerson{Home: "Köln", City: "Münster", Km: 125, Near: true}, 2, "Münster, about 125 km"},
		{"no city", FitPerson{Home: "Köln"}, 10, "City not known"},
		{"a city without a centre", FitPerson{Home: "Köln", City: "Hückeswagen"}, 10, "Hückeswagen, distance not known"},
		{"not set", FitPerson{City: "Bonn"}, 10, "Where you are is not set in Settings"},
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
		{2002, 10}, {1996, 10}, {1991, 5}, {1987, 1}, {1986, 0}, {1970, 0}, {0, 5},
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
		{[]string{"works_with_people", "athlete"}, 27, "Works with people, Athlete"},
		{[]string{"works_with_people", "athlete", "author", "runs_events"}, 40, "Works with people, Runs their own events, Author, Athlete"},
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
	if fit != sum || fit != 27+27+13+10 {
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

// A row shows FitPoints and the person Fit, so the two never disagree.
func TestFitPointsIsFit(t *testing.T) {
	now := day("2026-11-21")
	signals := [][]Signal{nil, {{Key: "works_with_people", For: true}}, {{Key: "athlete", For: true}, {Key: "funded"}}}
	dates := []time.Time{{}, day("2026-11-20"), day("2026-09-01"), day("2026-05-01")}
	n := 0
	for _, sigs := range signals {
		for _, born := range []int{0, 2001, 1992, 1985} {
			for _, active := range dates {
				for _, wrote := range dates {
					for _, known := range []bool{false, true} {
						for _, near := range []FitPerson{{}, {Home: "Köln", City: "Bonn", Km: 24.3, Near: true}, {Home: "Köln", City: "Köln", Near: true}, {Home: "Köln", City: "Hückeswagen"}} {
							p := near
							p.Signals, p.BornAbout, p.AgeReason, p.LastActive, p.LastWrote, p.Wrote, p.Now = sigs, born, "Bachelor from 2019", active, wrote, known, now
							if fit, parts := Fit(p); FitPoints(p) != fit {
								t.Fatalf("FitPoints %d, Fit %d %v for %+v", FitPoints(p), fit, parts, p)
							}
							n++
						}
					}
				}
			}
		}
	}
	if n < 1000 {
		t.Errorf("only %d people compared", n)
	}
}

// Tim, 22 November 2026: people who showed nothing beyond running a
// business stood at the top for being young. What they do counts most,
// then that they are active and write, then how near, and last how young.
func TestFitDoingComesBeforeYoung(t *testing.T) {
	now := day("2026-11-22")
	young := FitPerson{BornAbout: 2001, AgeReason: "First job from 2023", LastActive: day("2026-11-04"),
		Home: "Köln", City: "Düsseldorf", Km: 36, Near: true, Now: now}
	doing := young
	doing.BornAbout, doing.Signals = 1988, []Signal{{Key: "works_with_people", For: true}}
	doing.LastWrote, doing.Wrote = day("2026-11-04"), true
	if a, b := FitPoints(young), FitPoints(doing); a >= b {
		t.Errorf("about 25 and doing nothing known: %d, about 38, coaching and writing: %d", a, b)
	}
	_, parts := Fit(young)
	var order []string
	for _, p := range parts {
		order = append(order, p.Key)
	}
	if got := strings.Join(order, " "); got != "who active near younger" {
		t.Errorf("the parts come as %s", got)
	}
}
