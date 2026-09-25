package pipeline

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// advance updates a source after a successful check and moves it through
// its lifecycle:
//
//   - candidate: after its first check it goes on probation
//   - probation: active once a check keeps an event with named people,
//     retired when the probation weeks pass without one
//   - active: retired after too many checks in a row without a kept event
//   - retired: back on probation when it keeps an event again
func (p *Pipeline) advance(ctx context.Context, src Source, cfg settings.Settings, st ResolveStats, mode fetch.Mode) error {
	now := p.now()
	empty := src.Empty + 1
	if st.EventsKept > 0 {
		empty = 0
	}
	status := src.Status
	switch src.Status {
	case "candidate":
		status = "probation"
		if st.KeptWithPeople > 0 {
			status = "active"
		}
	case "probation":
		switch {
		case st.KeptWithPeople > 0:
			status = "active"
		case now.Sub(src.Changed) > time.Duration(cfg.Int("probation_weeks", 3))*7*24*time.Hour:
			status = "retired"
		}
	case "active":
		if empty >= cfg.Int("retire_after_empty_checks", 4) {
			status = "retired"
		}
	case "retired":
		if st.EventsKept > 0 {
			status = "probation"
		}
	}
	// A little slack, so tomorrow's run at the same hour finds it due.
	interval := cfg.Days("active_check_interval_days", 1)
	if status == "retired" {
		interval = cfg.Days("retired_recheck_days", 28)
	}
	next := now.Add(interval - 2*time.Hour)

	newMode := src.Mode
	if src.Mode == "auto" && st.EventsFound > 0 {
		newMode = string(mode)
	}
	_, err := p.Pool.Exec(ctx, `
		UPDATE sources SET
			checks = checks + 1, empty_checks_in_row = $2, last_checked_at = $3, next_check_at = $4,
			fetch_mode = $5, status = $6,
			status_changed_at = CASE WHEN status = $6 THEN status_changed_at ELSE $3 END,
			updated_at = $3
		WHERE id = $1`, src.ID, empty, now, next, newMode, status)
	return err
}

var meetupEventPath = regexp.MustCompile(`^/([^/]+)/events(?:/.*)?$`)

// SourceKindFor guesses a source's kind from its address.
func SourceKindFor(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "listing"
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch {
	case strings.HasSuffix(host, "lu.ma") || strings.HasSuffix(host, "luma.com"):
		if strings.Contains(u.Path, "/ics/") {
			return "calendar_ical"
		}
		return "calendar_luma"
	case host == "meetup.com":
		return "calendar_meetup"
	case strings.HasPrefix(host, "eventbrite."):
		return "calendar_eventbrite"
	case strings.HasSuffix(u.Path, ".ics") || strings.Contains(u.RawQuery, "ical"):
		return "calendar_ical"
	}
	return "listing"
}

// CalendarURL turns an event link on a platform into the organiser's
// calendar, which lists all their future dates.
func CalendarURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "meetup.com" {
		path := u.Path
		if i := strings.Index(path[1:], "/"); strings.HasPrefix(path, "/de-de/") || strings.HasPrefix(path, "/en-us/") {
			path = path[i+1:]
		}
		if m := meetupEventPath.FindStringSubmatch(path); m != nil {
			return "https://www.meetup.com/" + m[1] + "/"
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 1 && parts[0] != "" && parts[0] != "find" && parts[0] != "topics" && parts[0] != "cities" {
			return "https://www.meetup.com/" + parts[0] + "/"
		}
		return ""
	}
	u.Fragment = ""
	if (strings.HasSuffix(host, "luma.com") || strings.HasSuffix(host, "lu.ma") || strings.HasPrefix(host, "eventbrite.")) && !strings.Contains(u.Path, "/ics/") {
		u.RawQuery = ""
	}
	return u.String()
}

// discover adds the organiser calendars a page links to as candidate
// sources. It returns how many were new.
func (p *Pipeline) discover(ctx context.Context, src Source, links []string) (int, error) {
	added := 0
	for i, l := range links {
		if i >= 20 {
			break
		}
		cal := CalendarURL(l)
		if cal == "" || cal == src.URL || fetch.CheckAllowed(cal) != nil {
			continue
		}
		tag, err := p.Pool.Exec(ctx, `
			INSERT INTO sources (name, kind, url, status, discovered_from_source_id, discovered_note)
			VALUES ($1, $2, $3, 'candidate', $4, $5)
			ON CONFLICT (url) WHERE url IS NOT NULL DO NOTHING`,
			NameFromURL(cal), SourceKindFor(cal), cal, src.ID, "Linked from "+src.Name)
		if err != nil {
			return added, err
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}

func NameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	path := strings.Trim(u.Path, "/")
	if path == "" || strings.Contains(u.Path, "/ics/") {
		return host
	}
	return host + "/" + path
}
