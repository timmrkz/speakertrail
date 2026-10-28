package rubric_test

import (
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// Every step and place here is invented.
func TestBornAbout(t *testing.T) {
	for _, c := range []struct {
		name   string
		steps  []rubric.Step
		year   int
		reason string
	}{
		{"nothing known", nil, 0, ""},
		{"a bachelor's start", []rubric.Step{{Education: true, Title: "Bachelor of Science, Sportwissenschaft", From: 2018, To: 2021}}, 1999, "Bachelor from 2018"},
		{"a master's end without a start", []rubric.Step{{Education: true, Title: "M.Sc. Psychologie", To: 2024}}, 1999, "Master until 2024"},
		{"the Abitur", []rubric.Step{{Education: true, Title: "Abitur", Org: "Beispiel-Gymnasium", From: 2010, To: 2018}}, 1999, "School until 2018"},
		{"a late degree does not make someone younger", []rubric.Step{
			{Education: true, Title: "Bachelor of Arts", From: 2021, To: 2024},
			{Title: "Inhaberin", Org: "Studio Beispiel", From: 2015},
			{Title: "Verkäuferin", Org: "Muster Mode", From: 2008, To: 2015},
		}, 1986, "First job from 2008"},
		{"one job held now is no first job", []rubric.Step{{Title: "Founder", Org: "Probe Labs", From: 2023}}, 0, ""},
		{"a student job counts younger", []rubric.Step{
			{Title: "Gründer", Org: "Probe Labs", From: 2023},
			{Title: "Werkstudent Marketing", Org: "Beispiel AG", From: 2019, To: 2022},
		}, 2000, "Student job from 2019"},
		{"an international job is not an internship", []rubric.Step{
			{Title: "International Sales Manager", Org: "Beispiel AG", From: 2015},
			{Title: "Sales Manager", Org: "Muster GmbH", From: 2012, To: 2015},
		}, 1990, "First job from 2012"},
		{"a course says nothing", []rubric.Step{{Education: true, Title: "Ausbildung zur Yogalehrerin", From: 2022, To: 2023}}, 0, ""},
		{"a Diplom-Kauffrau studied", []rubric.Step{{Education: true, Title: "Diplom-Kauffrau", From: 2001, To: 2006}}, 1978, "Master from 2001"},
	} {
		t.Run(c.name, func(t *testing.T) {
			year, reason := rubric.BornAbout(c.steps)
			if year != c.year || reason != c.reason {
				t.Errorf("born about %d (%q), want %d (%q)", year, reason, c.year, c.reason)
			}
		})
	}
}

// Likely under 30 is a signal for, with the years that show it. Older is
// not counted against.
func TestYoung(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	s := rubric.Signals(rubric.Person{BornAbout: 1999, AgeReason: "Bachelor from 2018", Now: now})
	if len(s) != 1 || s[0].Key != "young" || !s[0].For || s[0].Passage != "Bachelor from 2018, so about 27" || s[0].Where != "LinkedIn profile" {
		t.Errorf("signals %+v", s)
	}
	if s := rubric.Signals(rubric.Person{BornAbout: 1996, AgeReason: "First job from 2018", Now: now}); len(s) != 0 {
		t.Errorf("about 30: %+v", s)
	}
}
