package extract

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// nextData returns the parsed __NEXT_DATA__ object of a Next.js page.
func nextData(doc string) map[string]any {
	for _, raw := range scripts(doc, func(typ, id string) bool { return id == "__NEXT_DATA__" }) {
		var v map[string]any
		if json.Unmarshal([]byte(raw), &v) == nil {
			return v
		}
	}
	return nil
}

// dig follows a path of object keys.
func dig(v any, path ...string) any {
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// luma reads Luma event, calendar and city pages. The data sits in
// __NEXT_DATA__ under props.pageProps.initialData.data. A calendar's full
// list comes from its iCal feed, which is followed.
func luma(p Page, r *Result) {
	data := dig(nextData(p.Body), "props", "pageProps", "initialData", "data")
	if data == nil {
		return
	}
	add := func(ev any, hosts any) {
		e, ok := lumaEvent(ev, hosts)
		if ok {
			r.Events = append(r.Events, e)
		}
	}
	if ev := dig(data, "event"); ev != nil {
		add(ev, dig(data, "hosts"))
	}
	for _, item := range asList(dig(data, "featured_items")) {
		add(dig(item, "event"), dig(item, "hosts"))
	}
	for _, item := range asList(dig(data, "events")) {
		if ev := dig(item, "event"); ev != nil {
			add(ev, dig(item, "hosts"))
		} else {
			add(item, nil)
		}
	}
	if id := str(dig(data, "calendar", "api_id")); strings.HasPrefix(id, "cal-") {
		r.Follow = append(r.Follow, "https://api.lu.ma/ics/get?entity=calendar&id="+url.QueryEscape(id))
	}
	if id := str(dig(data, "event", "calendar_api_id")); strings.HasPrefix(id, "cal-") {
		r.Links = append(r.Links, "https://api.lu.ma/ics/get?entity=calendar&id="+url.QueryEscape(id))
	}
}

func lumaEvent(v any, hosts any) (Event, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return Event{}, false
	}
	e := Event{Title: str(m["name"]), Method: "luma"}
	var okStart bool
	if e.Start, _, okStart = ParseTime(str(m["start_at"])); !okStart {
		return e, false
	}
	e.End, _, _ = ParseTime(str(m["end_at"]))
	if slug := str(m["url"]); slug != "" {
		if strings.HasPrefix(slug, "http") {
			e.URL = slug
		} else {
			e.URL = "https://luma.com/" + strings.TrimPrefix(slug, "/")
		}
	}
	geo, _ := m["geo_address_info"].(map[string]any)
	if geo != nil {
		e.Venue = str(geo["address"])
		e.Address = str(geo["full_address"])
		if e.Address == "" {
			e.Address = str(geo["short_address"])
		}
		e.City = CanonicalCity(str(geo["city"]))
	}
	switch strings.ToLower(str(m["location_type"])) {
	case "online", "zoom", "meet":
		e.Format = "online"
	case "offline":
		e.Format = "in_person"
	}
	for _, h := range asList(hosts) {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		name := str(hm["name"])
		if !LooksLikeName(name) {
			// A host can be the organising community itself.
			if name != "" {
				e.Organisers = append(e.Organisers, Organiser{Name: name})
			}
			continue
		}
		per := Person{Name: cleanName(name), Role: "host", Affiliation: str(hm["bio_short"])}
		if len(per.Affiliation) > 120 {
			per.Affiliation = ""
		}
		if w := str(hm["website"]); w != "" {
			per.Links = append(per.Links, w)
		}
		if li := str(hm["linkedin_handle"]); li != "" {
			per.Links = append(per.Links, "https://www.linkedin.com"+ensureSlash(li))
		}
		e.People = append(e.People, per)
	}
	return e, e.Title != ""
}

func ensureSlash(s string) string {
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/in/" + s
}

// meetup reads Meetup group and event pages from their Apollo cache in
// __NEXT_DATA__ (props.pageProps.__APOLLO_STATE__).
func meetup(p Page, r *Result) {
	state, _ := dig(nextData(p.Body), "props", "pageProps", "__APOLLO_STATE__").(map[string]any)
	if state == nil {
		return
	}
	ref := func(v any) map[string]any {
		key := str(dig(v, "__ref"))
		m, _ := state[key].(map[string]any)
		return m
	}
	for key, v := range state {
		if !strings.HasPrefix(key, "Event:") {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if s := strings.ToUpper(str(m["status"])); s == "CANCELLED" || s == "DELETED" {
			continue
		}
		e := Event{Title: str(m["title"]), URL: str(m["eventUrl"]), Description: StripHTML(str(m["description"])), Method: "meetup"}
		var ok2 bool
		if e.Start, _, ok2 = ParseTime(str(m["dateTime"])); !ok2 {
			continue
		}
		e.End, _, _ = ParseTime(str(m["endTime"]))
		switch strings.ToUpper(str(m["eventType"])) {
		case "ONLINE":
			e.Format = "online"
		case "PHYSICAL":
			e.Format = "in_person"
		case "HYBRID":
			e.Format = "hybrid"
		}
		if b, ok := m["isOnline"].(bool); ok && b && e.Format == "" {
			e.Format = "online"
		}
		if venue := ref(m["venue"]); venue != nil {
			e.Venue = str(venue["name"])
			e.Address = strings.Trim(strings.Join([]string{str(venue["address"]), str(venue["city"])}, ", "), ", ")
			e.City = CanonicalCity(str(venue["city"]))
		}
		if g := ref(m["group"]); g != nil {
			org := Organiser{Name: str(g["name"])}
			if u := str(g["urlname"]); u != "" {
				org.URL = "https://www.meetup.com/" + u + "/"
			}
			e.Organisers = append(e.Organisers, org)
		}
		for _, s := range asList(m["speakerDetails"]) {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			if n := str(sm["name"]); LooksLikeName(n) {
				e.People = append(e.People, Person{Name: cleanName(n), Role: "speaker", Affiliation: str(sm["description"])})
			}
		}
		r.Events = append(r.Events, e)
	}
	// Event pages also carry JSON-LD with details the cache may lack.
	if len(r.Events) == 0 {
		r.Events = JSONLD(p.Body, p.URL)
	}
}

// eventbrite reads JSON-LD on event and search pages and the server data of
// search and organiser pages.
func eventbrite(p Page, r *Result) {
	r.Events = JSONLD(p.Body, p.URL)
	if len(r.Events) > 0 {
		return
	}
	m := serverData.FindStringSubmatch(p.Body)
	if m == nil {
		return
	}
	var data map[string]any
	if json.Unmarshal([]byte(m[1]), &data) != nil {
		return
	}
	var list []any
	for _, path := range [][]string{{"search_data", "events", "results"}, {"view_data", "events", "future_events"}, {"events"}} {
		if l := asList(dig(data, path...)); len(l) > 0 {
			list = l
			break
		}
	}
	for _, it := range list {
		em, ok := it.(map[string]any)
		if !ok {
			continue
		}
		e := Event{Title: str(em["name"]), URL: str(em["url"]), Description: str(em["summary"]), Method: "eventbrite"}
		if n, ok := em["name"].(map[string]any); ok {
			e.Title = str(n["text"])
		}
		date, tm := str(em["start_date"]), str(em["start_time"])
		start := date
		if tm != "" {
			start = date + "T" + tm
		}
		if s := str(dig(em, "start", "local")); s != "" {
			start = s
		}
		var ok2 bool
		if e.Start, e.AllDay, ok2 = ParseTime(start); !ok2 {
			continue
		}
		if v, ok := em["primary_venue"].(map[string]any); ok {
			e.Venue = str(v["name"])
			e.Address = str(dig(v, "address", "localized_address_display"))
			e.City = CanonicalCity(str(dig(v, "address", "city")))
		}
		if b, ok := em["is_online_event"].(bool); ok && b {
			e.Format = "online"
		}
		r.Events = append(r.Events, e)
	}
}

var serverData = regexp.MustCompile(`(?s)window\.__SERVER_DATA__\s*=\s*(\{.*?\})\s*;\s*</script>`)

var platformLink = regexp.MustCompile(`https?://(?:www\.)?(?:luma\.com|lu\.ma|meetup\.com|eventbrite\.[a-z.]+/o|tickettailor\.com/events)/[A-Za-z0-9_\-./%]+`)

// platformLinks finds links to event calendars on other platforms, which
// are candidates for new sources.
func platformLinks(doc, base string) []string {
	var out []string
	for _, m := range platformLink.FindAllString(doc, 200) {
		u := absURL(base, strings.TrimRight(m, ".\"'"))
		if u == "" || u == base {
			continue
		}
		out = append(out, u)
	}
	return out
}
