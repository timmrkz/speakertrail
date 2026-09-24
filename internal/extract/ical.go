package extract

import (
	"strings"
	"time"
)

// ParseICal reads the VEVENTs of an iCalendar feed (RFC 5545).
func ParseICal(body, base string) []Event {
	// Unfold continuation lines.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n ", "")
	body = strings.ReplaceAll(body, "\n\t", "")

	var events []Event
	var cur *Event
	var calTZ *time.Location
	for _, line := range strings.Split(body, "\n") {
		nameParams, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name, params := nameParams, ""
		if i := strings.IndexByte(nameParams, ';'); i >= 0 {
			name, params = nameParams[:i], nameParams[i+1:]
		}
		name = strings.ToUpper(name)
		switch {
		case name == "X-WR-TIMEZONE":
			if loc, err := time.LoadLocation(strings.TrimSpace(value)); err == nil {
				calTZ = loc
			}
		case name == "BEGIN" && strings.EqualFold(value, "VEVENT"):
			cur = &Event{Method: "ical"}
		case name == "END" && strings.EqualFold(value, "VEVENT"):
			if cur != nil && cur.Title != "" && !cur.Start.IsZero() {
				events = append(events, *cur)
			}
			cur = nil
		case cur == nil:
			continue
		case name == "SUMMARY":
			cur.Title = icalText(value)
		case name == "DESCRIPTION":
			cur.Description = StripHTML(icalText(value))
		case name == "LOCATION":
			loc := icalText(value)
			if strings.HasPrefix(loc, "http") {
				if cur.Format == "" {
					cur.Format = "online"
				}
				continue
			}
			venue, addr, _ := strings.Cut(loc, ",")
			if addr == "" {
				cur.Address = strings.TrimSpace(venue)
			} else {
				cur.Venue, cur.Address = strings.TrimSpace(venue), strings.TrimSpace(addr)
			}
		case name == "URL":
			cur.URL = strings.TrimSpace(value)
		case name == "DTSTART":
			cur.Start, cur.AllDay = icalTime(value, params, calTZ)
		case name == "DTEND":
			cur.End, _ = icalTime(value, params, calTZ)
			if cur.AllDay && !cur.End.IsZero() {
				// An all-day DTEND is exclusive.
				cur.End = cur.End.Add(-time.Second)
			}
		case name == "ORGANIZER":
			if cn := paramValue(params, "CN"); cn != "" {
				cur.Organisers = append(cur.Organisers, Organiser{Name: cn})
			}
		case name == "STATUS" && strings.EqualFold(strings.TrimSpace(value), "CANCELLED"):
			cur.Title = ""
		}
	}
	return events
}

func icalText(v string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return strings.TrimSpace(r.Replace(v))
}

func paramValue(params, key string) string {
	for _, p := range strings.Split(params, ";") {
		k, v, ok := strings.Cut(p, "=")
		if ok && strings.EqualFold(k, key) {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func icalTime(value, params string, calTZ *time.Location) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(paramValue(params, "VALUE"), "DATE") || len(value) == 8 {
		t, err := time.ParseInLocation("20060102", value, Berlin)
		return t, err == nil
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false
		}
		return t, false
	}
	loc := Berlin
	if calTZ != nil {
		loc = calTZ
	}
	if tzid := paramValue(params, "TZID"); tzid != "" {
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	}
	t, err := time.ParseInLocation("20060102T150405", value, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, false
}
