// Package extract turns a fetched page into events and the people named on
// them. It tries the cheapest reliable method first: iCal feeds, schema.org
// Event data (JSON-LD), then platform adapters for Luma, Meetup and
// Eventbrite. Names in free text are read only from clearly marked lines
// such as "Speaker: ..." or "Auf der Bühne: ...".
package extract

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

// Event is one event found on a page.
type Event struct {
	Title       string
	Description string
	Start       time.Time
	// End is zero when unknown.
	End    time.Time
	AllDay bool
	// Venue is the place's name, Address the street address.
	Venue   string
	Address string
	City    string
	// Format is in_person, online, hybrid or empty when unknown.
	Format     string
	Type       string
	URL        string
	Price      string
	Organisers []Organiser
	People     []Person
	// Method names how the event was found, for the Runs screen.
	Method string
}

// Organiser is an organisation or person running an event.
type Organiser struct {
	Name string
	URL  string
}

// Person is someone named on an event.
type Person struct {
	Name string
	// Role is speaker, panelist, pitch, host or moderator.
	Role        string
	Affiliation string
	// Links are profile or website links found next to the name.
	Links []string
}

// Result is everything found on one page.
type Result struct {
	Events []Event
	// Follow lists feeds worth loading to complete this page, such as a
	// Luma calendar's iCal feed.
	Follow []string
	// Links are organiser and calendar pages found on the page, candidates
	// for new sources.
	Links []string
}

// Page is the input: what was fetched and from where.
type Page struct {
	URL         string
	ContentType string
	Body        string
}

// Berlin is the time zone for times without an offset.
var Berlin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return loc
}()

// Extract finds events on a page.
func Extract(p Page) Result {
	var r Result
	body := strings.TrimSpace(p.Body)
	if strings.Contains(p.ContentType, "calendar") || strings.HasPrefix(body, "BEGIN:VCALENDAR") {
		r.Events = ParseICal(body, p.URL)
		return finish(r, p.URL)
	}

	host := hostOf(p.URL)
	switch {
	case hostIs(host, "luma.com", "lu.ma"):
		luma(p, &r)
	case hostIs(host, "meetup.com"):
		meetup(p, &r)
	case hostIs(host, "eventbrite.de", "eventbrite.com", "eventbrite.co.uk", "eventbrite.at", "eventbrite.ch"):
		eventbrite(p, &r)
	}
	if len(r.Events) == 0 {
		r.Events = JSONLD(p.Body, p.URL)
	}
	r.Links = append(r.Links, platformLinks(p.Body, p.URL)...)
	return finish(r, p.URL)
}

// finish fills in what every method needs: absolute URLs, a city, a type,
// people from the description, and drops duplicates.
func finish(r Result, base string) Result {
	seen := map[string]bool{}
	var out []Event
	for _, e := range r.Events {
		e.Title = cleanText(e.Title)
		if e.Title == "" || e.Start.IsZero() {
			continue
		}
		e.URL = absURL(base, e.URL)
		e.Description = strings.TrimSpace(e.Description)
		if e.City == "" {
			e.City = CityOf(e.Address, e.Venue)
		}
		if e.Format == "" {
			e.Format = guessFormat(e)
		}
		if e.Type == "" {
			e.Type = GuessType(e.Title + "\n" + e.Description)
		}
		e.People = mergePeople(e.People, PeopleFromText(e.Description))
		for i := range e.Organisers {
			e.Organisers[i].Name = cleanText(e.Organisers[i].Name)
			e.Organisers[i].URL = absURL(base, e.Organisers[i].URL)
		}
		key := e.URL + "|" + e.Start.UTC().Format(time.RFC3339) + "|" + strings.ToLower(e.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	r.Events = out
	r.Links = uniqueStrings(r.Links)
	r.Follow = uniqueStrings(r.Follow)
	return r
}

func guessFormat(e Event) string {
	text := strings.ToLower(e.Title + " " + e.Venue + " " + e.Address)
	online := strings.Contains(text, "online") || strings.Contains(text, "webinar") || strings.Contains(text, "zoom") || strings.Contains(text, "virtual")
	switch {
	case online && (e.Address != "" || e.City != ""):
		return "hybrid"
	case online:
		return "online"
	case e.Address != "" || e.Venue != "" || e.City != "":
		return "in_person"
	}
	return ""
}

// mergePeople adds people from b to a unless a already names them.
func mergePeople(a, b []Person) []Person {
	have := map[string]bool{}
	for _, p := range a {
		have[NormaliseName(p.Name)] = true
	}
	for _, p := range b {
		if k := NormaliseName(p.Name); !have[k] {
			have[k] = true
			a = append(a, p)
		}
	}
	return a
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func hostIs(host string, domains ...string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// absURL resolves ref against base and drops tracking parameters and the
// fragment.
func absURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	u, err := b.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.Fragment = ""
	q := u.Query()
	changed := false
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "fbclid" || lk == "gclid" || lk == "aff" || lk == "ref" {
			q.Del(k)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// cleanText collapses white space and decodes stray HTML entities.
func cleanText(s string) string {
	s = htmlUnescape(s)
	return strings.Join(strings.Fields(s), " ")
}
