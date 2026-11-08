package rubric_test

import (
	"strings"
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

// The roles a profile says someone holds now show whether they run their
// own business. All invented.
func TestCurrentRoles(t *testing.T) {
	for _, c := range []struct {
		name, roles, want string
	}{
		{"an owner", "Inhaberin, Studio Beispiel", "+owner_operator: Inhaberin, Studio Beispiel"},
		{"a founder", "Gründerin, Probe Labs", "+owner_operator: Gründerin, Probe Labs"},
		{"a company named after her", "Systemischer Coach, Erika Beispiel Coaching", "+works_with_people: Systemischer Coach, Erika Beispiel Coaching | +owner_operator: Systemischer Coach, Erika Beispiel Coaching"},
		{"a freelancer", "Freelance Designerin", "+owner_operator: Freelance Designerin"},
		{"a corporate title, at a company that shares her name", "Head of Marketing, Beispiel AG", "-corporate: Head of Marketing, Beispiel AG"},
		{"a product owner", "Product Owner, Muster GmbH", ""},
		{"the second of two roles", "Teamleiterin, Muster GmbH · Mitgründerin, Probe Yoga", "+works_with_people: Mitgründerin, Probe Yoga | +owner_operator: Mitgründerin, Probe Yoga"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, s := range rubric.Signals(rubric.Person{Name: "Erika Beispiel", CurrentRoles: c.roles}) {
				sign := "+"
				if !s.For {
					sign = "-"
				}
				if s.Where != "LinkedIn profile" {
					t.Errorf("%s found in %q", s.Key, s.Where)
				}
				got = append(got, sign+s.Key+": "+s.Passage)
			}
			if g := strings.Join(got, " | "); g != c.want {
				t.Errorf("signals %q, want %q", g, c.want)
			}
		})
	}
}

// A guest runs their own business, and nothing rules them out. All
// invented.
func TestGuest(t *testing.T) {
	for _, c := range []struct {
		name, headline, roles string
		ok                    bool
		why                   string
	}{
		{"a trainer on her own", "Personal Trainerin", "Personal Trainerin, Selbstständig", true, ""},
		{"a founder, with an @", "Gründerin @ Studio Beispiel", "Founder & Managing Director, Studio Beispiel", true, ""},
		{"the managing partner of his gym", "", "Geschäftsführender Gesellschafter, Probe Gym GmbH", true, ""},
		{"a management consultant who founded his firm", "Management Consultant | Strategy", "Founder, Beispiel Consulting", false, "Consultant, agency or office"},
		{"an Unternehmensberater", "Unternehmensberater und Gründer", "Inhaber, Muster Beratung", false, "Consultant, agency or office"},
		{"a candidate for parliament", "Landtagskandidat Köln", "Gründer, Probe Events", false, "Consultant, agency or office"},
		{"a psychological counsellor is no consultant", "Psychologische Beraterin und Coach", "Inhaberin, Praxis Beispiel", true, ""},
		{"a nutrition coach is no consultant", "Ernährungsberaterin", "Selbstständige Ernährungsberaterin", true, ""},
		{"a trainer employed by a club", "Athletiktrainer", "Athletiktrainer, Probe Sportverein", false, "Does not run their own business"},
		{"a head of marketing", "Head of Marketing", "Gründerin, Nebenprojekt Beispiel", false, "Corporate title"},
		{"a former founder", "Former founder & software engineer", "Software Engineer, Probe Labs", false, "Does not run their own business"},
		{"an ehemaliger Gründer", "Ehemaliger Gründer, jetzt Produktmanager", "", false, "Does not run their own business"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ok, why := rubric.Guest(rubric.Signals(rubric.Person{Name: "Erika Beispiel", Headline: c.headline, CurrentRoles: c.roles}))
			if ok != c.ok || why != c.why {
				t.Errorf("guest %v (%q), want %v (%q)", ok, why, c.ok, c.why)
			}
		})
	}
}

// A profile's activity counts like a post, and the newer of the two wins.
func TestActiveOnLinkedIn(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	post := &rubric.Post{Title: "Ein Jahr selbstständig", Published: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}
	for _, c := range []struct {
		name   string
		active time.Time
		post   *rubric.Post
		want   string
	}{
		{"active last week", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), nil, "+active_on_linkedin: Last active on 1 November 2026"},
		{"quiet since spring", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), nil, "-quiet_on_linkedin: Last active on 1 March 2026"},
		{"active after an old post", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), post, "+active_on_linkedin: Last active on 1 October 2026"},
		{"an old post and no activity", time.Time{}, post, "-quiet_on_linkedin: Last post on 1 March 2026: “Ein Jahr selbstständig”"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, s := range rubric.Signals(rubric.Person{LastActive: c.active, LastPost: c.post, Now: now}) {
				sign := "+"
				if !s.For {
					sign = "-"
				}
				got = append(got, sign+s.Key+": "+s.Passage)
			}
			if g := strings.Join(got, " | "); g != c.want {
				t.Errorf("signals %q, want %q", g, c.want)
			}
		})
	}
}
