package extract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/extract"
)

func load(t *testing.T, name, pageURL string) extract.Result {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	ct := "text/html"
	if strings.HasSuffix(name, ".ics") {
		ct = "text/calendar"
	}
	return extract.Extract(extract.Page{URL: pageURL, ContentType: ct, Body: string(b)})
}

func berlin(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, extract.Berlin)
	if err != nil {
		panic(err)
	}
	return t
}

type wantEvent struct {
	title, city, venue, format, typ, url, price string
	start                                       time.Time
	organisers                                  []string
	people                                      []string // "Name|role|affiliation"
}

func check(t *testing.T, got []extract.Event, want []wantEvent) {
	t.Helper()
	if len(got) != len(want) {
		var titles []string
		for _, e := range got {
			titles = append(titles, e.Title)
		}
		t.Fatalf("got %d events %q, want %d", len(got), titles, len(want))
	}
	for i, w := range want {
		e := got[i]
		errorf := func(field string, g, w any) { t.Errorf("event %d (%s): %s = %q, want %q", i, w, field, g, w) }
		if e.Title != w.title {
			errorf("title", e.Title, w.title)
		}
		if !e.Start.Equal(w.start) {
			t.Errorf("event %d (%s): start %v, want %v", i, w.title, e.Start, w.start)
		}
		if w.city != "" && e.City != w.city {
			errorf("city", e.City, w.city)
		}
		if w.venue != "" && e.Venue != w.venue {
			errorf("venue", e.Venue, w.venue)
		}
		if w.format != "" && e.Format != w.format {
			errorf("format", e.Format, w.format)
		}
		if w.typ != "" && e.Type != w.typ {
			errorf("type", e.Type, w.typ)
		}
		if w.url != "" && e.URL != w.url {
			errorf("url", e.URL, w.url)
		}
		if w.price != "" && e.Price != w.price {
			errorf("price", e.Price, w.price)
		}
		var orgs []string
		for _, o := range e.Organisers {
			orgs = append(orgs, o.Name)
		}
		if w.organisers != nil && strings.Join(orgs, ",") != strings.Join(w.organisers, ",") {
			t.Errorf("event %d (%s): organisers %q, want %q", i, w.title, orgs, w.organisers)
		}
		var people []string
		for _, p := range e.People {
			people = append(people, p.Name+"|"+p.Role+"|"+p.Affiliation)
		}
		if strings.Join(people, "\n") != strings.Join(w.people, "\n") {
			t.Errorf("event %d (%s): people\n%s\nwant\n%s", i, w.title, strings.Join(people, "\n"), strings.Join(w.people, "\n"))
		}
	}
}

func TestJSONLDEventList(t *testing.T) {
	r := load(t, "events_calendar_list.html", "https://www.rheinlandpitch.de/events/list/")
	check(t, r.Events, []wantEvent{
		{
			title: "Rheinland Pitch #129", start: berlin("2026-09-28 18:00"), city: "Köln", venue: "Startplatz",
			format: "in_person", typ: "pitch", price: "free",
			url:        "https://www.rheinlandpitch.de/event/rheinland-pitch-129/",
			organisers: []string{"Rheinland Pitch"},
			people:     []string{"Lea Beispiel|pitch|Beispiel Robotics", "Jonas Muster|pitch|Kaffeekreis GmbH", "Petra von Testhausen|moderator|"},
		},
		{
			title: "Rheinland Pitch Düsseldorf #45", start: berlin("2026-10-06 18:30"), city: "Düsseldorf", venue: "TechHub K67",
			typ: "pitch", organisers: []string{"Rheinland Pitch"},
			people: []string{"Dr. Anna-Lena Probe|speaker|Probe Ventures"},
		},
	})
	if len(r.Links) != 1 || r.Links[0] != "https://www.meetup.com/beispiel-founders-koeln/" {
		t.Errorf("links %v", r.Links)
	}
}

func TestEventbriteEvent(t *testing.T) {
	r := load(t, "eventbrite_event.html", "https://www.eventbrite.de/e/female-founders-breakfast-essen-tickets-1234567890")
	check(t, r.Events, []wantEvent{{
		title: "Female Founders Breakfast Essen", start: berlin("2026-10-08 08:30"), city: "Essen", venue: "Unperfekthaus",
		format: "in_person", typ: "meetup", price: "free to € 15.00",
		url:        "https://www.eventbrite.de/e/female-founders-breakfast-essen-tickets-1234567890",
		organisers: []string{"Gründerinnen Ruhr"},
		people:     []string{"Maria Probst|speaker|Nordlicht Kosmetik", "Sabine Fiktiv|speaker|", "Aylin Erfunden|speaker|Code & Coffee"},
	}})
}

func TestLumaEvent(t *testing.T) {
	r := load(t, "luma_event.html", "https://luma.com/offline-evening-ehrenfeld")
	check(t, r.Events, []wantEvent{{
		title: "Offline Evening Ehrenfeld", start: berlin("2026-10-01 19:00"), city: "Köln", format: "in_person",
		url:        "https://luma.com/offline-evening-ehrenfeld",
		organisers: []string{"The Offline Circle"},
		people:     []string{"Clara Erfunden|host|Founder, The Offline Circle"},
	}})
	links := r.Events[0].People[0].Links
	if len(links) != 2 || links[1] != "https://www.linkedin.com/in/clara-erfunden" {
		t.Errorf("host links %v", links)
	}
	if len(r.Links) != 1 || !strings.Contains(r.Links[0], "cal-XYZ789") {
		t.Errorf("calendar link %v", r.Links)
	}
}

func TestLumaCalendarFollowsItsFeed(t *testing.T) {
	r := load(t, "luma_calendar.html", "https://luma.com/offlinecircle")
	check(t, r.Events, []wantEvent{
		{title: "Offline Evening Ehrenfeld", start: berlin("2026-10-01 19:00"), city: "Köln", people: []string{"Clara Erfunden|host|"}},
		{title: "Offline Walk Düsseldorf", start: berlin("2026-10-11 11:00"), city: "Düsseldorf", url: "https://luma.com/offline-walk"},
	})
	if len(r.Follow) != 1 || r.Follow[0] != "https://api.lu.ma/ics/get?entity=calendar&id=cal-XYZ789" {
		t.Errorf("follow %v", r.Follow)
	}
}

func TestMeetupGroup(t *testing.T) {
	r := load(t, "meetup_group.html", "https://www.meetup.com/beispiel-producttank-koeln/")
	check(t, r.Events, []wantEvent{
		{
			title: "Career changes in uncertain times", start: berlin("2026-09-28 19:00"), city: "Köln", venue: "Yello",
			format: "in_person", typ: "panel", organisers: []string{"Beispiel Product Tank Köln"},
			people: []string{"Mira Testfrau|speaker|Head of Product", "Tobias Probemann|panelist|Beispiel Cloud"},
		},
		{title: "Online AMA with a product leader", start: berlin("2026-10-05 18:00"), format: "online"},
	})
}

func TestICalFeed(t *testing.T) {
	r := load(t, "feed.ics", "https://example.org/calendar.ics")
	check(t, r.Events, []wantEvent{
		{
			title: "Gründer Abend Münster", start: berlin("2026-10-14 19:00"), city: "Münster", venue: "REACH Start-up Center",
			url: "https://example.org/gruender-abend", organisers: []string{"münster gründet"},
			people: []string{"Felix Ausgedacht|speaker|Solar Beispiel", "Hanna Probe|speaker|"},
		},
		{title: "Webinar Businessplan", start: berlin("2026-10-15 18:00"), format: "online", typ: "workshop"},
		{title: "Gründungstage", start: berlin("2026-11-09 00:00"), city: "Münster"},
	})
	if !r.Events[2].AllDay || r.Events[2].End.Day() != 11 {
		t.Errorf("all-day event: allDay %v end %v", r.Events[2].AllDay, r.Events[2].End)
	}
}

func TestPeopleFromText(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"Auf der Bühne: Lea Beispiel, Gründerin von Beispiel Robotics", []string{"Lea Beispiel|speaker|Gründerin von Beispiel Robotics"}},
		{"Speakers:\n- Jonas Muster (Kaffeekreis)\n- Dr. Petra Probe, CTO at Beispiel AG\n\nTickets: 10 Euro", []string{"Jonas Muster|speaker|Kaffeekreis", "Dr. Petra Probe|speaker|CTO at Beispiel AG"}},
		{"Moderation: Anna von der Probe", []string{"Anna von der Probe|moderator|"}},
		{"Jury: Max Erfunden, Nina Fiktiv und Ole Testmann", []string{"Max Erfunden|panelist|", "Nina Fiktiv|panelist|", "Ole Testmann|panelist|"}},
		{"Gastgeberin: Lisa-Marie Beispiel", []string{"Lisa-Marie Beispiel|host|"}},
		{"Wir freuen uns auf einen Vortrag von Sven Ausgedacht.", []string{"Sven Ausgedacht|speaker|"}},
		// Things that are not people stay out.
		{"Speaker: tba", nil},
		{"Startups: Beispiel Robotics GmbH, Kaffeekreis UG", nil},
		{"Ort: Startplatz, Köln\nUhrzeit: 18:00 Uhr", nil},
		{"Mit dabei: viele tolle Gründerinnen und Gründer", nil},
		{"Speaker: Köln Business Team", nil},
	}
	for _, c := range cases {
		var got []string
		for _, p := range extract.PeopleFromText(c.text) {
			got = append(got, p.Name+"|"+p.Role+"|"+p.Affiliation)
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("PeopleFromText(%q)\n got %q\nwant %q", c.text, got, c.want)
		}
	}
}

func TestNormaliseName(t *testing.T) {
	for in, want := range map[string]string{
		"Dr. Jürgen  Müller":  "jurgen muller",
		"JÜRGEN MÜLLER":       "jurgen muller",
		"Prof. Dr. Anna Groß": "anna gross",
		"Alejandro Peña":      "alejandro pena",
		"Lisa-Marie Beispiel": "lisa-marie beispiel",
	} {
		if got := extract.NormaliseName(in); got != want {
			t.Errorf("NormaliseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCityAndType(t *testing.T) {
	for in, want := range map[string]string{
		"Im Mediapark 5, 50670 Köln":     "Köln",
		"Cologne, Germany":               "Köln",
		"Friedrich-Ebert-Str. 18, Essen": "Essen",
		"Hauptstr. 1, 12345 Berlin":      "Berlin",
		"Mülheim an der Ruhr":            "Mülheim an der Ruhr",
		"Kölner Straße 5, Hürth":         "Hürth",
		"":                               "",
	} {
		if got := extract.CityOf(in); got != want {
			t.Errorf("CityOf(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Rheinland Pitch #129":         "pitch",
		"Startup Breakfast Köln":       "meetup",
		"Fuckup Nights Cologne Vol. 9": "talk",
		"Female Founders Summit":       "conference",
		"KI-Sprechstunde":              "workshop",
		"Webinar: Businessplan":        "workshop",
		"Afterwork\nwith a panel":      "meetup",
	} {
		if got := extract.GuessType(in); got != want {
			t.Errorf("GuessType(%q) = %q, want %q", in, got, want)
		}
	}
}
