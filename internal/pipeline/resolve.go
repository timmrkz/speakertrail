package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/extract"
)

// ResolveStats counts what one source check produced.
type ResolveStats struct {
	EventsFound int
	EventsKept  int
	EventsNew   int
	// KeptWithPeople counts kept events that name at least one person,
	// which is what moves a source from probation to active.
	KeptWithPeople int
	PeopleFound    int
	PeopleNew      int
}

// Source is the part of a source row the pipeline needs.
type Source struct {
	ID       int64
	Name     string
	URL      string
	City     string
	Status   string
	Mode     string
	Checks   int
	Empty    int
	Changed  time.Time
	Category string
}

// Resolver writes extracted events into the database, merging them with
// what is already known.
type Resolver struct {
	Pool  *pgxpool.Pool
	Rules FitRules
	// Now is the check time. Tests set it.
	Now func() time.Time
}

// Resolve stores the events of one check. Each event is written in its own
// short transaction, so no transaction stays open for long.
func (r *Resolver) Resolve(ctx context.Context, src Source, events []extract.Event) (ResolveStats, error) {
	var st ResolveStats
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	for _, e := range events {
		if e.City == "" && src.City != "" && e.Format != "online" {
			e.City = src.City
		}
		// A listing without links per event gives every event the page's
		// own address. That is no identity.
		if e.URL == src.URL {
			e.URL = ""
		}
		st.EventsFound++
		var kept, isNew bool
		var people, newPeople int
		err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
			var err error
			kept, isNew, people, newPeople, err = r.resolveEvent(ctx, tx, src, e, now)
			return err
		})
		if err != nil {
			return st, fmt.Errorf("resolve %q: %w", e.Title, err)
		}
		if kept {
			st.EventsKept++
			if people > 0 {
				st.KeptWithPeople++
			}
		}
		if isNew {
			st.EventsNew++
		}
		st.PeopleFound += people
		st.PeopleNew += newPeople
	}
	return st, nil
}

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// NormaliseTitle folds a title for duplicate checks.
func NormaliseTitle(t string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(t), " "), " ")
}

func (r *Resolver) resolveEvent(ctx context.Context, tx pgx.Tx, src Source, e extract.Event, now time.Time) (kept, isNew bool, people, newPeople int, err error) {
	norm := NormaliseTitle(e.Title)
	kept, reason := r.Rules.Fit(e)
	fit := "dropped"
	if kept {
		fit = "kept"
	}
	var end *time.Time
	if !e.End.IsZero() {
		end = &e.End
	}

	id, err := findEvent(ctx, tx, e.URL, norm, e.Start)
	if err != nil {
		return
	}
	if id == 0 {
		isNew = true
		err = tx.QueryRow(ctx, `
			INSERT INTO events (title, normalised_title, starts_at, ends_at, address, city, format, type,
				canonical_url, price, fit, fit_reason, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
			RETURNING id`,
			e.Title, norm, e.Start, end, e.Address, e.City, orDefault(e.Format, "in_person"), orDefault(e.Type, "other"),
			e.URL, e.Price, fit, reason, e.Description, now).Scan(&id)
		if err != nil {
			return
		}
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE events SET
				title = $2, normalised_title = $3, starts_at = $4,
				ends_at = COALESCE($5, ends_at),
				address = CASE WHEN $6 = '' THEN address ELSE $6 END,
				city = CASE WHEN $7 = '' THEN city ELSE $7 END,
				format = COALESCE(NULLIF($8, ''), format),
				type = CASE WHEN $9 = 'other' THEN type ELSE $9 END,
				canonical_url = CASE WHEN canonical_url = '' THEN $10 ELSE canonical_url END,
				price = CASE WHEN $11 = '' THEN price ELSE $11 END,
				description = CASE WHEN length($14) > length(description) THEN $14 ELSE description END,
				fit = CASE WHEN fit_manual THEN fit ELSE $12 END,
				fit_reason = CASE WHEN fit_manual THEN fit_reason ELSE $13 END,
				updated_at = $15
			WHERE id = $1`,
			id, e.Title, norm, e.Start, end, e.Address, e.City, e.Format, e.Type, e.URL, e.Price, fit, reason, e.Description, now)
		if err != nil {
			return
		}
		// Tim's decision wins.
		var manualFit string
		if err = tx.QueryRow(ctx, `SELECT fit FROM events WHERE id = $1`, id).Scan(&manualFit); err != nil {
			return
		}
		kept = manualFit == "kept"
	}
	if err = sight(ctx, tx, src.ID, now, "event_id", id, isNew); err != nil {
		return
	}

	if e.Venue != "" {
		var venue int64
		if venue, _, err = upsertOrganisation(ctx, tx, e.Venue, "venue", "", e.City); err != nil {
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE events SET venue_id = $2 WHERE id = $1 AND venue_id IS NULL`, id, venue); err != nil {
			return
		}
		if err = link(ctx, tx, `INSERT INTO organisings (organisation_id, event_id, role) VALUES ($1, $2, 'venue') ON CONFLICT DO NOTHING`, venue, id); err != nil {
			return
		}
	}
	for _, o := range e.Organisers {
		if o.Name == "" {
			continue
		}
		var org int64
		var orgNew bool
		if org, orgNew, err = upsertOrganisation(ctx, tx, o.Name, kindForURL(o.URL), o.URL, e.City); err != nil {
			return
		}
		if err = link(ctx, tx, `INSERT INTO organisings (organisation_id, event_id, role) VALUES ($1, $2, 'organiser') ON CONFLICT DO NOTHING`, org, id); err != nil {
			return
		}
		if err = sight(ctx, tx, src.ID, now, "organisation_id", org, orgNew); err != nil {
			return
		}
	}
	for _, p := range e.People {
		var pid int64
		var pNew bool
		if pid, pNew, err = r.resolvePerson(ctx, tx, id, e.City, p, e.URL, now); err != nil {
			return
		}
		people++
		if pNew {
			newPeople++
		}
		if err = sight(ctx, tx, src.ID, now, "person_id", pid, pNew); err != nil {
			return
		}
	}
	return
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// findEvent looks for the same event: the same canonical address, or the
// same start with a near-identical title.
func findEvent(ctx context.Context, tx pgx.Tx, canonical, normTitle string, start time.Time) (int64, error) {
	var id int64
	if canonical != "" {
		err := tx.QueryRow(ctx, `SELECT id FROM events WHERE canonical_url = $1`, canonical).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id, normalised_title FROM events WHERE starts_at = $1`, start)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			return 0, err
		}
		if sameTitle(t, normTitle) {
			return id, nil
		}
	}
	return 0, rows.Err()
}

// sameTitle treats titles as one when they are equal or one contains the
// other and the shorter one is not trivially short.
func sameTitle(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= 12 && strings.Contains(b, a)
}

func link(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	_, err := tx.Exec(ctx, sql, args...)
	return err
}

func sight(ctx context.Context, tx pgx.Tx, source int64, at time.Time, column string, id int64, isNew bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO sightings (source_id, checked_at, `+column+`, is_new) VALUES ($1, $2, $3, $4)`, source, at, id, isNew)
	return err
}

// NormaliseOrg folds an organisation name for duplicate checks.
func NormaliseOrg(name string) string {
	n := NormaliseTitle(name)
	for _, suffix := range []string{" gmbh", " ug", " ag", " e v", " ev", " kg", " inc", " ltd", " haftungsbeschrankt", " gbr"} {
		n = strings.TrimSuffix(n, suffix)
	}
	return strings.TrimSpace(n)
}

func upsertOrganisation(ctx context.Context, tx pgx.Tx, name, kind, website, city string) (int64, bool, error) {
	norm := NormaliseOrg(name)
	if norm == "" {
		return 0, false, errors.New("empty organisation name")
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM organisations WHERE normalised_name = $1 ORDER BY id LIMIT 1`, norm).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE organisations SET
				website = CASE WHEN website = '' THEN $2 ELSE website END,
				website_domain = CASE WHEN website_domain = '' THEN $3 ELSE website_domain END,
				city = CASE WHEN city = '' THEN $4 ELSE city END,
				kind = CASE WHEN kind = 'other' THEN $5 ELSE kind END,
				updated_at = now()
			WHERE id = $1`, id, website, domainOf(website), city, kind)
		return id, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO organisations (name, normalised_name, kind, city, website, website_domain)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		strings.TrimSpace(name), norm, kind, city, website, domainOf(website)).Scan(&id)
	return id, true, err
}

func kindForURL(u string) string {
	switch {
	case strings.Contains(u, "meetup.com/"):
		return "meetup_group"
	case strings.Contains(u, "luma.com/") || strings.Contains(u, "lu.ma/"):
		return "community"
	}
	return "other"
}

func domainOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// resolvePerson finds or creates a person. The same normalised name counts
// as the same person when they appear at the same event, when they share
// an organisation, or when neither side names a different organisation.
// Single-word names ("Nicola") only match within the same event.
func (r *Resolver) resolvePerson(ctx context.Context, tx pgx.Tx, eventID int64, city string, p extract.Person, eventURL string, now time.Time) (int64, bool, error) {
	norm := extract.NormaliseName(p.Name)
	role := extract.NormaliseRole(p.Role)
	orgName, affRole := ParseAffiliation(p.Affiliation)
	orgNorm := NormaliseOrg(orgName)

	var id int64
	err := tx.QueryRow(ctx, `
		SELECT p.id FROM people p JOIN appearances a ON a.person_id = p.id
		WHERE a.event_id = $1 AND p.normalised_name = $2 LIMIT 1`, eventID, norm).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	if id == 0 && strings.Contains(norm, " ") {
		rows, err := tx.Query(ctx, `
			SELECT p.id, COALESCE(array_agg(o.normalised_name) FILTER (WHERE o.id IS NOT NULL), '{}')
			FROM people p
			LEFT JOIN affiliations af ON af.person_id = p.id
			LEFT JOIN organisations o ON o.id = af.organisation_id
			WHERE p.normalised_name = $1
			GROUP BY p.id ORDER BY p.id`, norm)
		if err != nil {
			return 0, false, err
		}
		var fallback int64
		for rows.Next() {
			var cand int64
			var orgs []string
			if err := rows.Scan(&cand, &orgs); err != nil {
				rows.Close()
				return 0, false, err
			}
			shared := false
			for _, o := range orgs {
				if o == orgNorm && orgNorm != "" {
					shared = true
				}
			}
			if shared {
				id = cand
				break
			}
			if fallback == 0 && (orgNorm == "" || len(orgs) == 0) {
				fallback = cand
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, false, err
		}
		if id == 0 {
			id = fallback
		}
	}

	isNew := id == 0
	if isNew {
		err = tx.QueryRow(ctx, `
			INSERT INTO people (full_name, normalised_name, city, headline, created_at, updated_at, status_changed_at)
			VALUES ($1, $2, $3, $4, $5, $5, $5) RETURNING id`,
			p.Name, norm, city, p.Affiliation, now).Scan(&id)
		if err != nil {
			return 0, false, err
		}
	} else if p.Affiliation != "" {
		if _, err := tx.Exec(ctx, `UPDATE people SET headline = $2, updated_at = $3 WHERE id = $1 AND headline = ''`, id, p.Affiliation, now); err != nil {
			return 0, false, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO appearances (person_id, event_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, eventID, role); err != nil {
		return 0, false, err
	}
	if orgName != "" {
		org, _, err := upsertOrganisation(ctx, tx, orgName, "company", "", "")
		if err != nil {
			return 0, false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO affiliations (person_id, organisation_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, org, affRole); err != nil {
			return 0, false, err
		}
		if affRole == "founder" {
			if _, err := tx.Exec(ctx, `UPDATE people SET fit = 'founder' WHERE id = $1 AND fit = 'other'`, id); err != nil {
				return 0, false, err
			}
		}
	}
	for _, l := range p.Links {
		platform, clean := ProfileOf(l)
		if clean == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO profiles (person_id, platform, url, handle, found_via, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6)
			ON CONFLICT (platform, url) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at`,
			id, platform, clean, handleOf(clean), "event page "+eventURL, now); err != nil {
			return 0, false, err
		}
	}
	return id, isNew, nil
}

var (
	affAt = regexp.MustCompile(`(?i)^(.*?)\s+(?:at|bei|@|von)\s+(.+)$`)
	// founderish reads the role only, as whole words, so "Gründerzentrum"
	// or "Founders Foundation" in an organisation's name makes nobody a
	// founder. A CEO or managing director can run a bank, so they are not
	// founders by title alone.
	founderish = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(?:co-?founder|founder|founding partner|mitgründer(?:in)?|co-?gründer(?:in)?|gründer(?:in)?|inhaber(?:in)?|owner)(?:[^\p{L}]|$)`)
	roleish    = regexp.MustCompile(`(?i)(?:founder|gründer|ceo|cto|coo|cfo|head|lead|director|manager|leiter|partner|inhaber|owner|geschäftsführ|professor|student|designer|developer|engineer|consultant|berater|coach|author|autorin|autor|chair|vorstand|keramik|ceramicist|moderator|host)`)
)

// ParseAffiliation splits "Co-founder and CEO, JUPUS" or "Head of Product
// at Beispiel" into the organisation and a role for the affiliation. The
// role is founder only when the role part says so.
func ParseAffiliation(aff string) (org, role string) {
	aff = strings.TrimSpace(aff)
	if aff == "" {
		return "", ""
	}
	roleOf := func(part string) string {
		if founderish.MatchString(part) {
			return "founder"
		}
		return "employee"
	}
	if before, after, ok := strings.Cut(aff, ","); ok && roleish.MatchString(before) {
		return firstOrg(after), roleOf(before)
	}
	if m := affAt.FindStringSubmatch(aff); m != nil && roleish.MatchString(m[1]) {
		return firstOrg(m[2]), roleOf(m[1])
	}
	if roleish.MatchString(aff) && !orgLike.MatchString(aff) {
		// Only a role, like "Author".
		return "", roleOf(aff)
	}
	// Only an organisation's name.
	return firstOrg(aff), "employee"
}

// orgLike tells an organisation's name from a bare role, for names that
// contain a role word, like "Founders Foundation" or "Gründer-Stammtisch".
var orgLike = regexp.MustCompile(`(?i)(?:foundation|stiftung|stammtisch|zentrum|center|centre|club|verband|verein|e\.v\.|gmbh|\bug\b|\bag\b|network|netzwerk|allianz|campus|hub|lab)`)

func firstOrg(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ",;|"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(strings.Trim(s, ".()"))
	if len(s) < 2 || len(s) > 60 {
		return ""
	}
	return s
}

// ProfileOf names the platform of a profile link and cleans the link.
func ProfileOf(raw string) (platform, clean string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	u.RawQuery, u.Fragment = "", ""
	u.Host = strings.ToLower(u.Host)
	path := strings.TrimSuffix(u.Path, "/")
	switch {
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		return "linkedin", "https://www.linkedin.com" + path
	case host == "instagram.com":
		return "instagram", "https://www.instagram.com" + path
	case host == "youtube.com" || host == "youtu.be":
		return "youtube", "https://www.youtube.com" + path
	case host == "tiktok.com":
		return "tiktok", "https://www.tiktok.com" + path
	case host == "x.com" || host == "twitter.com":
		return "x", "https://x.com" + path
	case host == "luma.com" || host == "lu.ma":
		return "luma", "https://luma.com" + path
	case host == "meetup.com":
		return "meetup", "https://www.meetup.com" + path
	}
	u.Path = path
	return "website", u.String()
}

func handleOf(profile string) string {
	u, err := url.Parse(profile)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}
