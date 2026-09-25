package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// eventJSON builds one event as JSON. withPrivate adds the fields only Tim
// sees, showPeople is a SQL expression that decides whether names appear.
func eventJSON(withPrivate bool, showPeople string) string {
	personID := ""
	if withPrivate {
		personID = `'id', p.id,`
	}
	fields := fmt.Sprintf(`
		'id', e.id, 'title', e.title, 'starts_at', e.starts_at, 'ends_at', e.ends_at,
		'city', e.city, 'venue', COALESCE(v.name, ''), 'address', e.address,
		'format', e.format, 'type', e.type,
		'url', COALESCE(NULLIF(e.canonical_url, ''),
			(SELECT s.url FROM sightings si JOIN sources s ON s.id = si.source_id WHERE si.event_id = e.id ORDER BY si.id LIMIT 1), ''),
		'price', e.price,
		'organisers', (SELECT COALESCE(json_agg(o.name ORDER BY o.name), '[]') FROM organisings og
			JOIN organisations o ON o.id = og.organisation_id WHERE og.event_id = e.id AND og.role <> 'venue'),
		'people', CASE WHEN %s THEN (SELECT COALESCE(json_agg(json_build_object(%s 'name', p.full_name, 'role', a.role, 'affiliation', p.headline)
			ORDER BY a.role DESC, p.full_name), '[]') FROM appearances a JOIN people p ON p.id = a.person_id WHERE a.event_id = e.id)
			ELSE '[]'::json END`, showPeople, personID)
	if withPrivate {
		fields += `,
		'fit', COALESCE(e.fit, 'dropped'), 'fit_reason', e.fit_reason, 'fit_manual', e.fit_manual, 'first_seen', e.created_at,
		'sources', (SELECT COALESCE(json_agg(json_build_object('id', s.id, 'name', s.name, 'url', s.url) ORDER BY s.name), '[]')
			FROM sources s WHERE s.id IN (SELECT source_id FROM sightings WHERE event_id = e.id))`
	}
	return "json_build_object(" + fields + ")"
}

// window reads from and to as Berlin dates. to is inclusive.
func window(r *http.Request, cfg settings.Settings, now time.Time) (from, to time.Time, err error) {
	q := r.URL.Query()
	today := now.In(extract.Berlin)
	from = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, extract.Berlin)
	if v := q.Get("from"); v != "" {
		if from, err = time.ParseInLocation("2006-01-02", v, extract.Berlin); err != nil {
			return from, to, fmt.Errorf("from must be a date like 2026-09-28")
		}
	}
	to = from.AddDate(0, 0, cfg.Int("collect_ahead_days", 30))
	if v := q.Get("to"); v != "" {
		if to, err = time.ParseInLocation("2006-01-02", v, extract.Berlin); err != nil {
			return from, to, fmt.Errorf("to must be a date like 2026-10-25")
		}
	}
	return from, to.AddDate(0, 0, 1), nil
}

// publicConfig answers everyone, also while the calendar is closed, so the
// front page can say so. Tim, logged in, gets owner true and sees the
// calendar before it opens.
func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Load(r.Context(), s.opts.Pool)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	// The answer depends on the login, so no cache may keep it.
	w.Header().Set("Cache-Control", "no-cache")
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object(
			'public_calendar', $1::boolean,
			'owner', $2::boolean,
			'show_people', $3::boolean,
			'cities', CASE WHEN $1 OR $2 THEN (SELECT COALESCE(json_agg(city ORDER BY n DESC, city), '[]') FROM (
				SELECT city, count(*) AS n FROM events
				WHERE fit = 'kept' AND format <> 'online' AND starts_at >= $4 AND city <> ''
				GROUP BY city) c) ELSE '[]'::json END)`,
		cfg.Bool("public_calendar", false), s.sessions.loggedIn(r),
		cfg.Bool("public_show_people", false), s.opts.Now().Add(-12*time.Hour))
}

func (s *Server) publicEvents(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Load(r.Context(), s.opts.Pool)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	open := cfg.Bool("public_calendar", false)
	if !open && !s.sessions.loggedIn(r) {
		fail(w, http.StatusNotFound, "The calendar is not open yet")
		return
	}
	from, to, err := window(r, cfg, s.opts.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	if open {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	show := "false"
	if cfg.Bool("public_show_people", false) {
		show = "true"
	}
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('events', COALESCE(json_agg(`+eventJSON(false, show)+` ORDER BY e.starts_at, e.id), '[]'))
		FROM events e LEFT JOIN organisations v ON v.id = e.venue_id
		WHERE e.fit = 'kept' AND e.format <> 'online'
		  AND (e.starts_at >= $1 OR e.ends_at >= $1) AND e.starts_at < $2
		  AND ($3 = '' OR lower(e.city) = lower($3))
		  AND ($4 = '' OR e.type = $4)`,
		from, to, strings.TrimSpace(q.Get("city")), strings.TrimSpace(q.Get("type")))
}
