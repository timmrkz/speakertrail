package extract

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func htmlUnescape(s string) string { return html.UnescapeString(s) }

// unescapeTwice decodes descriptions whose HTML was escaped before being
// put into JSON, as some WordPress plugins do.
func unescapeTwice(s string) string {
	if strings.Contains(s, "&lt;") {
		return html.UnescapeString(s)
	}
	return s
}

// scripts returns the contents of every <script> whose type or id matches.
func scripts(doc string, match func(typ, id string) bool) []string {
	root, err := nethtml.Parse(strings.NewReader(doc))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(n *nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && n.DataAtom == atom.Script {
			var typ, id string
			for _, a := range n.Attr {
				switch a.Key {
				case "type":
					typ = strings.ToLower(strings.TrimSpace(a.Val))
				case "id":
					id = a.Val
				}
			}
			if match(typ, id) && n.FirstChild != nil {
				out = append(out, n.FirstChild.Data)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

var eventTypes = map[string]bool{
	"event": true, "businessevent": true, "educationevent": true, "socialevent": true, "festival": true,
	"exhibitionevent": true, "literaryevent": true, "sportsevent": true, "musicevent": true, "comedyevent": true,
	"theaterevent": true, "screeningevent": true, "danceevent": true, "foodevent": true, "hackathon": true,
	"visualartsevent": true, "courseinstance": true, "childrensevent": true, "publicationevent": true,
}

// JSONLD reads schema.org Event data from a page's JSON-LD blocks.
func JSONLD(doc, base string) []Event {
	var events []Event
	for _, raw := range scripts(doc, func(typ, _ string) bool { return typ == "application/ld+json" }) {
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
			// Some sites put several objects or trailing commas in one block.
			continue
		}
		for _, obj := range flattenLD(v) {
			if e, ok := eventFromLD(obj); ok {
				e.Method = "json-ld"
				events = append(events, e)
			}
		}
	}
	return events
}

// flattenLD walks arrays, @graph and ItemList wrappers down to objects.
func flattenLD(v any) []map[string]any {
	var out []map[string]any
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			out = append(out, flattenLD(x)...)
		}
	case map[string]any:
		if g, ok := t["@graph"]; ok {
			out = append(out, flattenLD(g)...)
		}
		if hasType(t, "itemlist") {
			if items, ok := t["itemListElement"]; ok {
				for _, it := range asList(items) {
					m, ok := it.(map[string]any)
					if !ok {
						continue
					}
					if inner, ok := m["item"]; ok {
						out = append(out, flattenLD(inner)...)
					} else {
						out = append(out, flattenLD(m)...)
					}
				}
			}
		}
		if isEvent(t) {
			out = append(out, t)
		}
		if sub, ok := t["subEvent"]; ok {
			out = append(out, flattenLD(sub)...)
		}
	}
	return out
}

func isEvent(m map[string]any) bool {
	for _, t := range types(m) {
		if eventTypes[t] {
			return true
		}
	}
	return false
}

func hasType(m map[string]any, want string) bool {
	for _, t := range types(m) {
		if t == want {
			return true
		}
	}
	return false
}

func types(m map[string]any) []string {
	var out []string
	for _, t := range asList(m["@type"]) {
		if s, ok := t.(string); ok {
			s = strings.ToLower(s)
			s = s[strings.LastIndexAny(s, "/:")+1:]
			out = append(out, s)
		}
	}
	return out
}

func asList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	default:
		return []any{t}
	}
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case map[string]any:
		if s := str(t["@value"]); s != "" {
			return s
		}
		if s := str(t["url"]); s != "" {
			return s
		}
		return str(t["@id"])
	case []any:
		if len(t) > 0 {
			return str(t[0])
		}
	}
	return ""
}

func eventFromLD(m map[string]any) (Event, bool) {
	e := Event{
		Title:       cleanText(str(m["name"])),
		Description: StripHTML(unescapeTwice(str(m["description"]))),
		URL:         str(m["url"]),
	}
	var ok bool
	if e.Start, e.AllDay, ok = ParseTime(str(m["startDate"])); !ok {
		return e, false
	}
	e.End, _, _ = ParseTime(str(m["endDate"]))
	if e.AllDay && !e.End.IsZero() {
		e.End = e.End.Add(24*time.Hour - time.Second)
	}

	mode := strings.ToLower(str(m["eventAttendanceMode"]))
	switch {
	case strings.Contains(mode, "mixed"):
		e.Format = "hybrid"
	case strings.Contains(mode, "online"):
		e.Format = "online"
	case strings.Contains(mode, "offline"):
		e.Format = "in_person"
	}
	virtual := false
	for _, l := range asList(m["location"]) {
		switch loc := l.(type) {
		case string:
			if e.Address == "" {
				e.Address = cleanText(loc)
			}
		case map[string]any:
			if hasType(loc, "virtuallocation") {
				virtual = true
				continue
			}
			if e.Venue == "" {
				e.Venue = cleanText(str(loc["name"]))
			}
			addr, city := addressOf(loc["address"])
			if e.Address == "" {
				e.Address = addr
			}
			if e.City == "" {
				e.City = CanonicalCity(city)
			}
		}
	}
	if virtual && e.Format == "" {
		if e.Venue != "" || e.Address != "" {
			e.Format = "hybrid"
		} else {
			e.Format = "online"
		}
	}

	for _, o := range asList(m["organizer"]) {
		switch t := o.(type) {
		case string:
			e.Organisers = append(e.Organisers, Organiser{Name: t})
		case map[string]any:
			if n := cleanText(str(t["name"])); n != "" {
				e.Organisers = append(e.Organisers, Organiser{Name: n, URL: str(t["url"])})
			}
		}
	}
	for _, p := range asList(m["performer"]) {
		t, ok := p.(map[string]any)
		if !ok {
			if s, ok := p.(string); ok && LooksLikeName(s) {
				e.People = append(e.People, Person{Name: cleanName(s), Role: "speaker"})
			}
			continue
		}
		n := cleanText(str(t["name"]))
		if hasType(t, "person") || (!hasType(t, "organization") && LooksLikeName(n)) {
			if !LooksLikeName(n) {
				continue
			}
			per := Person{Name: cleanName(n), Role: "speaker", Affiliation: cleanText(str(t["jobTitle"]))}
			if aff := str(t["affiliation"]); aff != "" {
				if per.Affiliation != "" {
					per.Affiliation += ", "
				}
				per.Affiliation += cleanText(aff)
			}
			if u := str(t["url"]); u != "" {
				per.Links = append(per.Links, u)
			}
			for _, s := range asList(t["sameAs"]) {
				if u := str(s); u != "" {
					per.Links = append(per.Links, u)
				}
			}
			e.People = append(e.People, per)
		}
	}
	e.Price = priceOf(m)
	return e, e.Title != ""
}

func addressOf(v any) (addr, city string) {
	switch a := v.(type) {
	case string:
		return cleanText(a), ""
	case map[string]any:
		street := cleanText(str(a["streetAddress"]))
		zip := cleanText(str(a["postalCode"]))
		city = cleanText(str(a["addressLocality"]))
		parts := []string{}
		if street != "" {
			parts = append(parts, street)
		}
		if zc := strings.TrimSpace(zip + " " + city); zc != "" {
			parts = append(parts, zc)
		}
		return strings.Join(parts, ", "), city
	}
	return "", ""
}

func priceOf(m map[string]any) string {
	if free, ok := m["isAccessibleForFree"].(bool); ok && free {
		return "free"
	}
	for _, o := range asList(m["offers"]) {
		off, ok := o.(map[string]any)
		if !ok {
			continue
		}
		cur := str(off["priceCurrency"])
		if cur == "EUR" || cur == "" {
			cur = "€"
		}
		low, high, price := str(off["lowPrice"]), str(off["highPrice"]), str(off["price"])
		switch {
		case low != "" && high != "" && low != high:
			if low == "0" || low == "0.00" {
				return fmt.Sprintf("free to %s %s", cur, high)
			}
			return fmt.Sprintf("%s %s to %s", cur, low, high)
		case price == "0" || price == "0.00" || low == "0" || low == "0.00":
			return "free"
		case price != "":
			return cur + " " + price
		case low != "":
			return cur + " " + low
		}
	}
	return ""
}

// ParseTime reads the date formats found in schema.org and platform data.
// Times without an offset are Berlin time. allDay is true for a bare date.
func ParseTime(s string) (t time.Time, allDay, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, false
	}
	withZone := []string{time.RFC3339Nano, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04Z07:00", "2006-01-02T15:04Z0700", "2006-01-02 15:04:05Z07:00"}
	for _, f := range withZone {
		if t, err := time.Parse(f, s); err == nil {
			return t, false, true
		}
	}
	local := []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"}
	for _, f := range local {
		if t, err := time.ParseInLocation(f, s, Berlin); err == nil {
			return t, false, true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, Berlin); err == nil {
		return t, true, true
	}
	return time.Time{}, false, false
}

// StripHTML turns an HTML fragment into plain text with line breaks.
func StripHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return strings.TrimSpace(s)
	}
	root, err := nethtml.Parse(strings.NewReader("<body>" + s + "</body>"))
	if err != nil {
		return s
	}
	var b strings.Builder
	var walk func(n *nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == nethtml.ElementNode && (n.DataAtom == atom.Br) {
			b.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == nethtml.ElementNode {
			switch n.DataAtom {
			case atom.P, atom.Div, atom.Li, atom.H1, atom.H2, atom.H3, atom.H4, atom.Tr, atom.Ul, atom.Ol:
				b.WriteByte('\n')
			}
		}
	}
	walk(root)
	lines := strings.Split(b.String(), "\n")
	var out []string
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
