package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/pipeline"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// runJSON builds one run with its totals from the checks, the reads of
// event pages and the lookups of startups. A run started by hand or by Check now is worked on by serve,
// which records no end. It is finished once none of its jobs wait any more.
const runJSON = `json_build_object(
	'id', r.id, 'kind', r.kind, 'started_at', r.started_at,
	'finished_at', COALESCE(r.finished_at, CASE WHEN NOT EXISTS (
		SELECT 1 FROM jobs j WHERE j.status IN ('queued', 'running') AND j.key LIKE 'run:' || r.id || ':%')
		THEN GREATEST(r.started_at, (SELECT max(checked_at) FROM source_checks WHERE run_id = r.id),
			(SELECT max(read_at) FROM event_reads WHERE run_id = r.id),
			(SELECT max(looked_up_at) FROM startup_lookups WHERE run_id = r.id)) END),
	'sources_checked', (SELECT count(DISTINCT source_id) FROM source_checks WHERE run_id = r.id),
	'events_found', (SELECT COALESCE(sum(events_found), 0) FROM source_checks WHERE run_id = r.id),
	'events_new', (SELECT COALESCE(sum(events_new), 0) FROM source_checks WHERE run_id = r.id),
	'people_new', (SELECT COALESCE(sum(people_new), 0) FROM source_checks WHERE run_id = r.id)
		+ (SELECT COALESCE(sum(people_new), 0) FROM event_reads WHERE run_id = r.id)
		+ (SELECT COALESCE(sum(people_new), 0) FROM startup_lookups WHERE run_id = r.id),
	'pages_read', (SELECT count(*) FROM event_reads WHERE run_id = r.id),
	'startups_looked_up', (SELECT count(*) FROM startup_lookups WHERE run_id = r.id),
	'progress', CASE WHEN r.finished_at IS NULL AND EXISTS (
		SELECT 1 FROM jobs j WHERE j.status IN ('queued', 'running') AND j.key LIKE 'run:' || r.id || ':%')
		THEN (` + progressJSON + `) END,
	'errors', (SELECT count(*) FROM source_checks WHERE run_id = r.id AND error <> ''))`

// progressJSON says how far a going run is: its checks, reads and lookups,
// done and in all, what is running now, and about how long is left. The
// time left is measured, from how long the last 200 checks, 100 reads and
// 100 lookups took. Checks and lookups run four at a time, reads one at a
// time, because the model answers one question at a time. Without any
// history there is no estimate.
const progressJSON = `WITH j AS (
		SELECT j.kind, j.status, s.kind AS source_kind FROM jobs j
		LEFT JOIN sources s ON j.kind = 'check_source' AND s.id = (j.payload->>'source_id')::bigint
		WHERE j.key LIKE 'run:' || r.id || ':%' AND j.kind IN ('check_source', 'read_event', 'look_up_startup')),
	c AS (SELECT
		count(*) FILTER (WHERE kind = 'check_source') AS checks,
		count(*) FILTER (WHERE kind = 'check_source' AND status IN ('done', 'failed')) AS checks_done,
		count(*) FILTER (WHERE kind = 'check_source' AND source_kind = 'portfolio' AND status NOT IN ('done', 'failed')) AS portfolios_left,
		count(*) FILTER (WHERE kind = 'read_event') AS reads,
		count(*) FILTER (WHERE kind = 'read_event' AND status IN ('done', 'failed')) AS reads_done,
		count(*) FILTER (WHERE kind = 'look_up_startup') AS lookups,
		count(*) FILTER (WHERE kind = 'look_up_startup' AND status IN ('done', 'failed')) AS lookups_done FROM j),
	speed AS (SELECT
		(SELECT avg(duration_ms) FROM (SELECT duration_ms FROM source_checks ORDER BY id DESC LIMIT 200) x) AS check_ms,
		(SELECT avg(duration_ms) FROM (SELECT duration_ms FROM event_reads WHERE error = '' ORDER BY id DESC LIMIT 100) x) AS read_ms,
		(SELECT avg(duration_ms) FROM (SELECT duration_ms FROM startup_lookups ORDER BY id DESC LIMIT 100) x) AS lookup_ms,
		-- Each check queues reads of its new events, so the run expects as
		-- many per check as checks brought lately, up to the setting.
		LEAST(COALESCE((SELECT (value #>> '{}')::numeric FROM settings WHERE key = 'event_pages_per_check'), 3),
			COALESCE((SELECT count(*) FROM event_reads WHERE read_at > now() - interval '14 days')::numeric
				/ NULLIF((SELECT count(*) FROM source_checks WHERE checked_at > now() - interval '14 days'), 0), 0)) AS reads_per_check,
		-- Each portfolio check queues up to this many lookups.
		COALESCE((SELECT (value #>> '{}')::numeric FROM settings WHERE key = 'startups_per_run'), 10) AS lookups_per_portfolio),
	expect AS (SELECT
		GREATEST(c.reads, c.reads + round((c.checks - c.checks_done) * speed.reads_per_check)) AS reads,
		c.lookups + c.portfolios_left * speed.lookups_per_portfolio AS lookups FROM c, speed)
	SELECT json_build_object(
		'checks', c.checks, 'checks_done', c.checks_done, 'reads', c.reads, 'reads_done', c.reads_done, 'reads_expected', expect.reads,
		'lookups', c.lookups, 'lookups_done', c.lookups_done, 'lookups_expected', expect.lookups,
		'now', (SELECT COALESCE(json_agg(json_build_object(
				'kind', CASE jr.kind WHEN 'check_source' THEN 'check' WHEN 'read_event' THEN 'read' ELSE 'lookup' END,
				'label', CASE jr.kind
					WHEN 'check_source' THEN (SELECT name FROM sources WHERE id = (jr.payload->>'source_id')::bigint)
					WHEN 'read_event' THEN (SELECT title FROM events WHERE id = (jr.payload->>'event_id')::bigint)
					ELSE (SELECT name FROM organisations WHERE id = (jr.payload->>'organisation_id')::bigint) END,
				'since', jr.updated_at) ORDER BY jr.updated_at), '[]')
			FROM jobs jr WHERE jr.status = 'running' AND jr.key LIKE 'run:' || r.id || ':%'
				AND jr.kind IN ('check_source', 'read_event', 'look_up_startup')),
		'seconds_left', CASE WHEN (c.checks > c.checks_done AND speed.check_ms IS NULL) OR (expect.reads > c.reads_done AND speed.read_ms IS NULL)
				OR (expect.lookups > c.lookups_done AND speed.lookup_ms IS NULL)
			THEN NULL ELSE round(GREATEST(
				((c.checks - c.checks_done) * COALESCE(speed.check_ms, 0) + (expect.lookups - c.lookups_done) * COALESCE(speed.lookup_ms, 0)) / 4,
				(expect.reads - c.reads_done) * COALESCE(speed.read_ms, 0)) / 1000) END)
	FROM c, speed, expect`

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	now := s.opts.Now()
	s.sendQuery(w, r, http.StatusOK, `
		WITH weeks AS (
			SELECT generate_series(date_trunc('week', ($1::timestamptz) AT TIME ZONE 'Europe/Berlin') - interval '7 weeks',
				date_trunc('week', ($1::timestamptz) AT TIME ZONE 'Europe/Berlin'), interval '1 week') AS week)
		SELECT json_build_object(
			'totals', json_build_object(
				'people', (SELECT count(*) FROM people),
				'organisations', (SELECT count(*) FROM organisations),
				'profiles', (SELECT count(*) FROM profiles WHERE review <> 'rejected'),
				'events_upcoming', (SELECT count(*) FROM events WHERE starts_at >= $2),
				'events_kept_upcoming', (SELECT count(*) FROM events WHERE starts_at >= $2 AND fit = 'kept'),
				'sources', (SELECT json_build_object(
					'active', count(*) FILTER (WHERE status = 'active'),
					'probation', count(*) FILTER (WHERE status = 'probation'),
					'candidate', count(*) FILTER (WHERE status = 'candidate'),
					'retired', count(*) FILTER (WHERE status = 'retired'),
					'manual', count(*) FILTER (WHERE status = 'manual')) FROM sources)),
			'last_7_days', json_build_object(
				'people', (SELECT count(*) FROM people WHERE created_at >= $3),
				'events', (SELECT count(*) FROM events WHERE created_at >= $3 AND fit = 'kept'),
				'organisations', (SELECT count(*) FROM organisations WHERE created_at >= $3),
				'sources', (SELECT count(*) FROM sources WHERE created_at >= $3 AND status IN ('active', 'probation', 'candidate'))),
			'weekly', (SELECT json_agg(json_build_object(
				'week_start', to_char(week, 'YYYY-MM-DD'),
				'people', (SELECT count(*) FROM people WHERE date_trunc('week', created_at AT TIME ZONE 'Europe/Berlin') = week),
				'events', (SELECT count(*) FROM events WHERE date_trunc('week', created_at AT TIME ZONE 'Europe/Berlin') = week),
				'sources', (SELECT count(*) FROM sources WHERE date_trunc('week', created_at AT TIME ZONE 'Europe/Berlin') = week))
				ORDER BY week) FROM weeks),
			'cities', (SELECT COALESCE(json_agg(json_build_object('city', city, 'events', n) ORDER BY n DESC, city), '[]') FROM (
				SELECT city, count(*) AS n FROM events WHERE starts_at >= $2 AND fit = 'kept' AND city <> ''
				GROUP BY city ORDER BY n DESC, city LIMIT 12) c),
			'last_run', (SELECT `+runJSON+` FROM runs r ORDER BY r.started_at DESC LIMIT 1))`,
		now, now.Add(-12*time.Hour), now.Add(-7*24*time.Hour))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Load(r.Context(), s.opts.Pool)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	from, to, err := window(r, cfg, s.opts.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	fit := q.Get("fit")
	if fit == "" {
		fit = "kept"
	}
	if fit != "kept" && fit != "dropped" && fit != "all" {
		fail(w, http.StatusBadRequest, "fit must be kept, dropped or all")
		return
	}
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('events', COALESCE(json_agg(`+eventJSON(true, "true")+` ORDER BY e.starts_at, e.id), '[]'))
		FROM events e LEFT JOIN organisations v ON v.id = e.venue_id
		WHERE (e.starts_at >= $1 OR e.ends_at >= $1) AND e.starts_at < $2
		  AND ($3 = 'all' OR e.fit = $3)
		  AND ($4 = '' OR lower(e.city) = lower($4))
		  AND ($5 = '' OR e.type = $5)
		  AND ($6 = '' OR e.title ILIKE '%' || $6 || '%' OR e.description ILIKE '%' || $6 || '%' OR v.name ILIKE '%' || $6 || '%'
		       OR EXISTS (SELECT 1 FROM appearances a JOIN people p ON p.id = a.person_id WHERE a.event_id = e.id AND p.full_name ILIKE '%' || $6 || '%'))`,
		from, to, fit, strings.TrimSpace(q.Get("city")), strings.TrimSpace(q.Get("type")), likeSafe(q.Get("q")))
}

// likeSafe escapes the wildcards of a search term.
func likeSafe(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(s))
}

func (s *Server) sendEvent(w http.ResponseWriter, r *http.Request, id int64) {
	s.sendQuery(w, r, http.StatusOK, `SELECT `+eventJSON(true, "true")+` FROM events e LEFT JOIN organisations v ON v.id = e.venue_id WHERE e.id = $1`, id)
}

func (s *Server) patchEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Fit       string `json:"fit"`
		FitReason string `json:"fit_reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Fit != "kept" && body.Fit != "dropped" {
		fail(w, http.StatusBadRequest, "fit must be kept or dropped")
		return
	}
	if body.FitReason == "" {
		body.FitReason = map[string]string{"kept": "Kept by Tim", "dropped": "Dropped by Tim"}[body.Fit]
	}
	tag, err := s.opts.Pool.Exec(r.Context(), `UPDATE events SET fit = $2, fit_reason = $3, fit_manual = true, updated_at = now() WHERE id = $1`, id, body.Fit, body.FitReason)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	s.sendEvent(w, r, id)
}

// personJSON builds the list fields of a person.
const personJSON = `json_build_object(
	'id', p.id, 'name', p.full_name, 'known_as', p.known_as, 'headline', p.headline, 'city', p.city, 'fit', p.fit,
	'first_seen', p.created_at,
	'appearances', (SELECT count(*) FROM appearances WHERE person_id = p.id),
	'next_appearance', (SELECT json_build_object('event_id', e.id, 'title', e.title, 'starts_at', e.starts_at, 'city', e.city, 'role', a.role)
		FROM appearances a JOIN events e ON e.id = a.event_id
		WHERE a.person_id = p.id AND e.starts_at >= $1 ORDER BY e.starts_at LIMIT 1),
	'profiles', (SELECT COALESCE(json_agg(json_build_object('id', pr.id, 'platform', pr.platform, 'url', pr.url, 'review', pr.review) ORDER BY pr.platform), '[]')
		FROM profiles pr WHERE pr.person_id = p.id))`

func (s *Server) people(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	order := map[string]string{
		"":     "next_start NULLS LAST, p.full_name",
		"next": "next_start NULLS LAST, p.full_name",
		"new":  "p.created_at DESC, p.id DESC",
		"name": "p.full_name",
	}[q.Get("sort")]
	if order == "" {
		fail(w, http.StatusBadRequest, "sort must be next, new or name")
		return
	}
	filter := map[string]string{
		"":         "true",
		"all":      "true",
		"upcoming": "next_start IS NOT NULL",
		"profile":  "EXISTS (SELECT 1 FROM profiles pr WHERE pr.person_id = p.id AND pr.review <> 'rejected')",
		"founder":  "p.fit = 'founder'",
	}[q.Get("filter")]
	if filter == "" {
		fail(w, http.StatusBadRequest, "filter must be all, upcoming, profile or founder")
		return
	}
	// counts says how many people each filter shows for the same search, so
	// a filter that shows nobody never hides that others were found.
	s.sendQuery(w, r, http.StatusOK, `
		WITH base AS (
			SELECT p.*, (SELECT min(e.starts_at) FROM appearances a JOIN events e ON e.id = a.event_id
					WHERE a.person_id = p.id AND e.starts_at >= $1) AS next_start
			FROM people p
			WHERE ($2 = '' OR p.full_name ILIKE '%' || $2 || '%' OR p.headline ILIKE '%' || $2 || '%' OR p.city ILIKE '%' || $2 || '%'))
		SELECT json_build_object(
			'people', (SELECT COALESCE(json_agg(`+personJSON+` ORDER BY `+order+`), '[]') FROM base p WHERE `+filter+`),
			'counts', (SELECT json_build_object(
				'all', count(*),
				'founder', count(*) FILTER (WHERE p.fit = 'founder'),
				'upcoming', count(*) FILTER (WHERE p.next_start IS NOT NULL),
				'profile', count(*) FILTER (WHERE EXISTS (SELECT 1 FROM profiles pr WHERE pr.person_id = p.id AND pr.review <> 'rejected')))
				FROM base p))`,
		s.opts.Now().Add(-12*time.Hour), likeSafe(q.Get("q")))
}

func (s *Server) sendPerson(w http.ResponseWriter, r *http.Request, id int64) {
	s.sendQuery(w, r, http.StatusOK, `
		SELECT (`+personJSON+`::jsonb || jsonb_build_object(
			'notes', p.notes,
			'fit_evidence', p.fit_evidence,
			'appearances', (SELECT COALESCE(jsonb_agg(jsonb_build_object('role', a.role, 'evidence', a.evidence, 'event', jsonb_build_object(
					'id', e.id, 'title', e.title, 'starts_at', e.starts_at, 'city', e.city, 'venue', COALESCE(v.name, ''),
					'url', e.canonical_url)) ORDER BY e.starts_at DESC), '[]')
				FROM appearances a JOIN events e ON e.id = a.event_id LEFT JOIN organisations v ON v.id = e.venue_id
				WHERE a.person_id = p.id),
			'affiliations', (SELECT COALESCE(jsonb_agg(jsonb_build_object('organisation', o.name, 'role', af.role, 'current', af.is_current) ORDER BY o.name), '[]')
				FROM affiliations af JOIN organisations o ON o.id = af.organisation_id WHERE af.person_id = p.id),
			'sightings', (SELECT COALESCE(jsonb_agg(jsonb_build_object('source_id', x.id, 'source', x.name, 'checked_at', x.last) ORDER BY x.last DESC), '[]')
				FROM (SELECT s.id, s.name, max(si.checked_at) AS last FROM sightings si JOIN sources s ON s.id = si.source_id
					WHERE si.person_id = p.id GROUP BY s.id, s.name) x),
			'appearances_count', (SELECT count(*) FROM appearances WHERE person_id = p.id)))::json
		FROM people p WHERE p.id = $2`, s.opts.Now().Add(-12*time.Hour), id)
}

func (s *Server) person(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		s.sendPerson(w, r, id)
	}
}

func (s *Server) patchPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Notes *string `json:"notes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Notes != nil {
		tag, err := s.opts.Pool.Exec(r.Context(), `UPDATE people SET notes = $2, updated_at = now() WHERE id = $1`, id, *body.Notes)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, http.StatusNotFound, "Not found")
			return
		}
	}
	s.sendPerson(w, r, id)
}

func (s *Server) patchProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Review string `json:"review"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Review != "open" && body.Review != "confirmed" && body.Review != "rejected" {
		fail(w, http.StatusBadRequest, "review must be open, confirmed or rejected")
		return
	}
	tag, err := s.opts.Pool.Exec(r.Context(), `UPDATE profiles SET review = $2 WHERE id = $1`, id, body.Review)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sourceJSON builds a source with its last check and health.
const sourceJSON = `json_build_object(
	'id', s.id, 'name', s.name, 'kind', s.kind, 'url', s.url, 'query', s.query, 'category', s.category, 'city', s.city,
	'status', s.status, 'fetch_mode', s.fetch_mode, 'notes', s.notes,
	'last_checked_at', s.last_checked_at, 'next_check_at', s.next_check_at, 'checks', s.checks,
	'empty_checks_in_row', s.empty_checks_in_row, 'points', s.points,
	'last_check', CASE WHEN lc.id IS NULL THEN NULL ELSE json_build_object(
		'events_found', lc.events_found, 'startups_found', lc.startups_found,
		'http_status', lc.http_status, 'mode', lc.mode, 'error', lc.error) END,
	'health', CASE
		WHEN s.status = 'manual' THEN 'warning'
		WHEN lc.id IS NULL THEN 'never'
		WHEN lc.error <> '' THEN 'error'
		WHEN lc.found = 0 THEN 'warning'
		WHEN prev.avg_found >= 4 AND lc.found < prev.avg_found * 0.3 THEN 'warning'
		ELSE 'ok' END,
	'health_note', CASE
		WHEN s.status = 'manual' THEN 'Followed by hand. The engine does not check it'
		WHEN lc.id IS NULL THEN ''
		WHEN lc.error <> '' THEN lc.error
		WHEN lc.found = 0 THEN 'The last check found no ' || CASE WHEN s.kind = 'portfolio' THEN 'startups' ELSE 'events' END
		WHEN prev.avg_found >= 4 AND lc.found < prev.avg_found * 0.3 THEN
			'Found ' || lc.found || CASE WHEN s.kind = 'portfolio' THEN ' startups' ELSE ' events' END || ', usually about ' || round(prev.avg_found)
		ELSE '' END,
	'discovered_from', COALESCE(
		(SELECT 'From the starting list: ' || left(input, 80) FROM seeds WHERE id = s.discovered_from_seed_id),
		(SELECT 'Linked from ' || name FROM sources x WHERE x.id = s.discovered_from_source_id),
		NULLIF(s.discovered_note, '')))`

// A check found events, or startups when the source is a portfolio.
const sourceFrom = `FROM sources s
	LEFT JOIN LATERAL (SELECT *, CASE WHEN s.kind = 'portfolio' THEN c.startups_found ELSE c.events_found END AS found
		FROM source_checks c WHERE c.source_id = s.id ORDER BY c.checked_at DESC, c.id DESC LIMIT 1) lc ON true
	LEFT JOIN LATERAL (SELECT avg(found) AS avg_found FROM (
		SELECT CASE WHEN s.kind = 'portfolio' THEN c.startups_found ELSE c.events_found END AS found
		FROM source_checks c WHERE c.source_id = s.id AND c.id <> lc.id AND c.error = ''
		ORDER BY c.checked_at DESC LIMIT 4) p) prev ON true`

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('sources', COALESCE(json_agg(`+sourceJSON+`
			ORDER BY array_position(ARRAY['active','probation','candidate','manual','retired'], s.status), s.points DESC, s.name), '[]'))
		`+sourceFrom+`
		WHERE ($1 = '' OR s.status = $1)
		  AND ($2 = '' OR s.name ILIKE '%' || $2 || '%' OR s.url ILIKE '%' || $2 || '%' OR s.city ILIKE '%' || $2 || '%' OR s.notes ILIKE '%' || $2 || '%')`,
		q.Get("status"), likeSafe(q.Get("q")))
}

func (s *Server) sendSource(w http.ResponseWriter, r *http.Request, status int, id int64) {
	s.sendQuery(w, r, status, `SELECT `+sourceJSON+` `+sourceFrom+` WHERE s.id = $1`, id)
}

func (s *Server) addSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL  string `json:"url"`
		Name string `json:"name"`
		// Portfolio is true for a page that lists startups, not events.
		Portfolio bool `json:"portfolio"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := url.Parse(strings.TrimSpace(body.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		fail(w, http.StatusBadRequest, "Enter a full web address that starts with https://")
		return
	}
	if errors.Is(fetch.CheckAllowed(u.String()), fetch.ErrBlockedHost) {
		fail(w, http.StatusBadRequest, "LinkedIn and Instagram are never checked. Add the organiser's website or event page instead")
		return
	}
	if host := strings.ToLower(u.Hostname()); host == "facebook.com" || strings.HasSuffix(host, ".facebook.com") {
		fail(w, http.StatusBadRequest, "Facebook pages cannot be checked. Add the organiser's website or event page instead")
		return
	}
	// A link to one event becomes the calendar it belongs to.
	link := u.String()
	kind := "portfolio"
	if !body.Portfolio {
		if cal := pipeline.CalendarURL(link); cal != "" {
			link = cal
		}
		kind = pipeline.SourceKindFor(link)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = pipeline.NameFromURL(link)
	}
	var id int64
	err = s.opts.Pool.QueryRow(r.Context(), `
		INSERT INTO sources (name, kind, url, status, discovered_note) VALUES ($1, $2, $3, 'candidate', 'Added by Tim')
		RETURNING id`, name, kind, link).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		fail(w, http.StatusConflict, "This address is already a source")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.sendSource(w, r, http.StatusCreated, id)
}

func (s *Server) patchSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status    *string `json:"status"`
		FetchMode *string `json:"fetch_mode"`
		Notes     *string `json:"notes"`
		Name      *string `json:"name"`
		// Portfolio switches between a page of startups and a page of
		// events.
		Portfolio *bool `json:"portfolio"`
	}
	if !decode(w, r, &body) {
		return
	}
	valid := map[string]bool{"candidate": true, "probation": true, "active": true, "retired": true, "manual": true}
	if body.Status != nil && !valid[*body.Status] {
		fail(w, http.StatusBadRequest, "status must be candidate, probation, active, retired or manual")
		return
	}
	if body.FetchMode != nil && *body.FetchMode != "auto" && *body.FetchMode != "http" && *body.FetchMode != "browser" {
		fail(w, http.StatusBadRequest, "fetch_mode must be auto, http or browser")
		return
	}
	var kind *string
	if body.Portfolio != nil {
		var link *string
		err := s.opts.Pool.QueryRow(r.Context(), `SELECT url FROM sources WHERE id = $1`, id).Scan(&link)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, http.StatusNotFound, "Not found")
			return
		}
		if err != nil {
			s.internal(w, r, err)
			return
		}
		if link == nil {
			fail(w, http.StatusBadRequest, "Only a source with a web address can list startups")
			return
		}
		k := "portfolio"
		if !*body.Portfolio {
			k = pipeline.SourceKindFor(*link)
		}
		kind = &k
	}
	tag, err := s.opts.Pool.Exec(r.Context(), `
		UPDATE sources SET
			kind = COALESCE($6, kind),
			status = COALESCE($2, status),
			status_changed_at = CASE WHEN $2::text IS NOT NULL AND $2 <> status THEN now() ELSE status_changed_at END,
			empty_checks_in_row = CASE WHEN $2::text IS NOT NULL AND $2 <> status THEN 0 ELSE empty_checks_in_row END,
			-- A source set going again, or that now lists something else, is
			-- checked in the next run.
			next_check_at = CASE WHEN ($2::text IN ('active', 'probation', 'candidate') AND $2 <> status)
				OR ($6::text IS NOT NULL AND $6 <> kind) THEN NULL ELSE next_check_at END,
			fetch_mode = COALESCE($3, fetch_mode),
			notes = COALESCE($4, notes),
			name = COALESCE(NULLIF($5, ''), name),
			updated_at = now()
		WHERE id = $1`, id, body.Status, body.FetchMode, body.Notes, body.Name, kind)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	s.sendSource(w, r, http.StatusOK, id)
}

func (s *Server) checkSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var exists bool
	if err := s.opts.Pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1 AND url IS NOT NULL)`, id).Scan(&exists); err != nil {
		s.internal(w, r, err)
		return
	}
	if !exists {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	run, err := s.opts.Pipeline.CheckNow(r.Context(), id)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"run_id": run})
}

// startRun starts a run by hand, or answers with the one still going.
func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	id, started, err := s.opts.Pipeline.RunNow(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": id, "started": started})
}

// currentRun answers the run that is going, or null, for the dot on Runs.
func (s *Server) currentRun(w http.ResponseWriter, r *http.Request) {
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('run', (SELECT `+runJSON+` FROM runs r
			WHERE r.kind <> 'check' AND r.finished_at IS NULL AND EXISTS (
				SELECT 1 FROM jobs j WHERE j.status IN ('queued', 'running') AND j.key LIKE 'run:' || r.id || ':%')
			ORDER BY r.id DESC LIMIT 1))`)
}

// stopRun ends a run by hand. A run that already ended answers 409.
func (s *Server) stopRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	stopped, err := s.opts.Pipeline.StopRun(r.Context(), id)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if !stopped {
		fail(w, http.StatusConflict, "This run has already ended")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('runs', COALESCE(json_agg(`+runJSON+` ORDER BY r.started_at DESC), '[]'))
		FROM (SELECT * FROM runs ORDER BY started_at DESC LIMIT 50) r`)
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('run', `+runJSON+`,
			'checks', (SELECT COALESCE(json_agg(json_build_object(
				'id', c.id, 'source', json_build_object('id', src.id, 'name', src.name, 'url', src.url),
				'mode', c.mode, 'http_status', c.http_status, 'events_found', c.events_found, 'events_kept', c.events_kept,
				'events_new', c.events_new, 'people_found', c.people_found, 'people_new', c.people_new,
				'error', c.error, 'duration_ms', c.duration_ms, 'checked_at', c.checked_at,
				'fetch_id', (SELECT f.id FROM fetches f WHERE f.id = c.fetch_id))
				ORDER BY (c.error <> '') DESC, c.events_found DESC, src.name), '[]')
				FROM source_checks c JOIN sources src ON src.id = c.source_id WHERE c.run_id = r.id))
		FROM runs r WHERE r.id = $1`, id)
}

func (s *Server) fetchPart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	column := map[string]string{"text": "visible_text", "html": "html", "screenshot": "screenshot"}[r.PathValue("part")]
	if column == "" {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	var data []byte
	err := s.opts.Pool.QueryRow(r.Context(), `SELECT `+column+` FROM fetches WHERE id = $1`, id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(data) == 0) {
		fail(w, http.StatusNotFound, "Not stored, or older than 30 days")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if column == "screenshot" {
		w.Header().Set("Content-Type", http.DetectContentType(data))
	} else {
		// Stored pages are shown as text. Served as HTML, a stranger's page
		// would run its scripts inside this interface.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Write(data)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('settings', json_agg(json_build_object(
			'key', key, 'value', value, 'description', description, 'updated_at', updated_at) ORDER BY key))
		FROM settings`)
}

func (s *Server) patchSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var body struct {
		Value json.RawMessage `json:"value"`
	}
	if !decode(w, r, &body) {
		return
	}
	var current []byte
	err := s.opts.Pool.QueryRow(r.Context(), `SELECT value FROM settings WHERE key = $1`, key).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "No such setting")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if kindOf(body.Value) != kindOf(current) {
		fail(w, http.StatusBadRequest, "This setting needs a "+kindOf(current))
		return
	}
	if _, err := s.opts.Pool.Exec(r.Context(), `UPDATE settings SET value = $2, updated_at = now() WHERE key = $1`, key, []byte(body.Value)); err != nil {
		s.internal(w, r, err)
		return
	}
	s.sendQuery(w, r, http.StatusOK, `
		SELECT json_build_object('key', key, 'value', value, 'description', description, 'updated_at', updated_at)
		FROM settings WHERE key = $1`, key)
}

// kindOf names the JSON type of a value, for validation.
func kindOf(raw []byte) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "invalid value"
	}
	switch t := v.(type) {
	case bool:
		return "true or false"
	case float64:
		return "number"
	case string:
		return "text"
	case []any:
		for _, x := range t {
			if _, ok := x.(string); !ok {
				return "list"
			}
		}
		return "list of texts"
	case map[string]any:
		return "set of named values"
	}
	return "value"
}
