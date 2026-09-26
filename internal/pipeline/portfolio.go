package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/queue"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// KindLookUp is a lookup: one look at one startup's website and its
// imprint, for the people who run it.
const KindLookUp = "look_up_startup"

// LookUpPayload is the payload of a look_up_startup job.
type LookUpPayload struct {
	OrganisationID int64 `json:"organisation_id"`
	RunID          int64 `json:"run_id"`
}

// maxStartups caps what one portfolio check stores.
const maxStartups = 300

// maxPortfolioPages caps the pages of one portfolio a check loads. They are
// on one website, one request every 5 seconds.
const maxPortfolioPages = 6

// finishPortfolio stores the startups a portfolio lists, one short
// transaction, and queues lookups of the ones not looked up yet.
func (p *Pipeline) finishPortfolio(ctx context.Context, src Source, runID int64, cfg settings.Settings, page *fetch.Page, rec checkRecord) error {
	base := page.FinalURL
	if base == "" {
		base = src.URL
	}
	startups := p.portfolioStartups(ctx, page.Body, base)
	if len(startups) > maxStartups {
		startups = startups[:maxStartups]
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, s := range startups {
		var id int64
		isNew := false
		var err error
		if s.Website != "" {
			err = tx.QueryRow(ctx, `SELECT id FROM organisations WHERE website_domain = $1 ORDER BY id LIMIT 1`, domainOf(s.Website)).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, `SELECT id FROM organisations WHERE portfolio_page = $1 ORDER BY id LIMIT 1`, s.Page).Scan(&id)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			id, isNew, err = upsertOrganisation(ctx, tx, s.Name, "company", s.Website, "")
			if err == nil && s.Page != "" {
				_, err = tx.Exec(ctx, `UPDATE organisations SET portfolio_page = $2 WHERE id = $1 AND portfolio_page = ''`, id, s.Page)
			}
		}
		if err != nil {
			return err
		}
		if err := sight(ctx, tx, src.ID, rec.checkedAt, "organisation_id", id, isNew); err != nil {
			return err
		}
		rec.startups++
		if isNew {
			rec.startupsN++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	rec.duration = p.now().Sub(rec.checkedAt)
	if err := p.recordCheck(ctx, src, runID, rec); err != nil {
		return err
	}
	if _, err := p.enqueueLookUps(ctx, runID, src.ID, cfg.Int("startups_per_run", 10)); err != nil {
		p.log().Warn("queueing lookups failed", "source", src.ID, "error", err)
	}
	return p.advancePortfolio(ctx, src, cfg, rec.startups, fetch.Mode(rec.mode))
}

// portfolioStartups reads a portfolio's first page and the pages after it,
// like "/p2", up to maxPortfolioPages, and lists each startup once.
func (p *Pipeline) portfolioStartups(ctx context.Context, body, base string) []extract.Startup {
	var out []extract.Startup
	seen := map[string]bool{base: true}
	queue := []string{}
	for pages := 1; ; pages++ {
		pp := extract.Portfolio(body, base)
		for _, s := range pp.Startups {
			key := s.Website + " " + s.Page
			if !seen[key] {
				seen[key] = true
				out = append(out, s)
			}
		}
		for _, m := range pp.More {
			if !seen[m] {
				seen[m] = true
				queue = append(queue, m)
			}
		}
		if len(queue) == 0 || pages >= maxPortfolioPages {
			return out
		}
		next := queue[0]
		queue = queue[1:]
		page, err := p.page(ctx, next)
		if err != nil {
			p.log().Info("a portfolio page failed", "url", next, "error", err)
			return out
		}
		body, base = page.Body, next
	}
}

// advancePortfolio moves a portfolio through its lifecycle. It is active
// while it lists startups and retired after too many checks without any.
// A portfolio changes slowly, so it is checked every two weeks.
func (p *Pipeline) advancePortfolio(ctx context.Context, src Source, cfg settings.Settings, found int, mode fetch.Mode) error {
	now := p.now()
	empty := src.Empty + 1
	status := src.Status
	switch {
	case found > 0:
		empty, status = 0, "active"
	case src.Status == "candidate":
		status = "probation"
	case src.Status == "probation" && now.Sub(src.Changed) > time.Duration(cfg.Int("probation_weeks", 3))*7*24*time.Hour,
		src.Status == "active" && empty >= cfg.Int("retire_after_empty_checks", 4):
		status = "retired"
	}
	interval := cfg.Days("portfolio_check_days", 14)
	if status == "retired" {
		interval = cfg.Days("retired_recheck_days", 28)
	}
	newMode := src.Mode
	if src.Mode == "auto" && found > 0 {
		newMode = string(mode)
	}
	_, err := p.Pool.Exec(ctx, `
		UPDATE sources SET
			checks = checks + 1, empty_checks_in_row = $2, last_checked_at = $3, next_check_at = $4,
			fetch_mode = $5, status = $6,
			status_changed_at = CASE WHEN status = $6 THEN status_changed_at ELSE $3 END,
			updated_at = $3
		WHERE id = $1`, src.ID, empty, now, now.Add(interval-2*time.Hour), newMode, status)
	return err
}

// unlookedStartups are startups from portfolios whose imprint was not looked
// up yet, then those looked up longest ago, before $3, to see whether they
// are still active. One whose site failed three times since is given up.
const unlookedStartups = `
	SELECT o.id FROM organisations o
	WHERE (o.looked_up_at IS NULL OR o.looked_up_at < $3) AND (o.website <> '' OR o.portfolio_page <> '')
	  AND EXISTS (SELECT 1 FROM sightings si JOIN sources s ON s.id = si.source_id
		WHERE si.organisation_id = o.id AND s.kind = 'portfolio' AND ($2 = 0 OR s.id = $2))
	  AND (SELECT count(*) FROM startup_lookups l WHERE l.organisation_id = o.id AND l.error <> ''
		AND l.looked_up_at > COALESCE(o.looked_up_at, '-infinity')) < 3
	ORDER BY o.looked_up_at NULLS FIRST, o.id LIMIT $1`

// relookupBefore is when a startup's last lookup is old enough for another.
func (p *Pipeline) relookupBefore(cfg settings.Settings) time.Time {
	return p.now().Add(-cfg.Days("relookup_days", 90))
}

// enqueueLookUps queues lookups for a run. With a source it only takes
// startups that portfolio lists.
func (p *Pipeline) enqueueLookUps(ctx context.Context, runID, sourceID int64, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return 0, err
	}
	rows, err := p.Pool.Query(ctx, unlookedStartups, limit, sourceID, p.relookupBefore(cfg))
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := p.Queue.Enqueue(ctx, queue.NewJob{
			Kind: KindLookUp, Key: fmt.Sprintf("run:%d:lookup:%d", runID, id),
			Payload: LookUpPayload{OrganisationID: id, RunID: runID},
		}); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (p *Pipeline) handleLookUp(ctx context.Context, j *queue.Job) error {
	var pl LookUpPayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	return p.LookUp(ctx, pl.OrganisationID, pl.RunID)
}

type lookUpRecord struct {
	runID, orgID int64
	url, imprint string
	website      string
	// profiles are links to people's own profiles found on the startup's
	// site. Only those that carry a founder's name are kept.
	profiles          []string
	note, err         string
	duration          time.Duration
	at                time.Time
	found, isNew      int
	sourceID          int64
	sourceName, name  string
	lookedUp, renamed bool
	// activity is what the signs of life say, see organisations.activity,
	// with lastSign the newest date the website shows.
	activity, activityNote string
	lastSign               *time.Time
}

// LookUp loads a startup's website, finds its imprint and stores the
// people it names as the ones who run the company. It also looks for signs
// of life: a parked domain, a company being wound up, and the newest date
// the site shows. A site that cannot be reached is tried again by a later
// run, and after three tries it counts as gone.
func (p *Pipeline) LookUp(ctx context.Context, orgID, runID int64) error {
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	rec := lookUpRecord{runID: runID, orgID: orgID, at: p.now()}
	var lookedUp *time.Time
	var portfolioPage string
	err = p.Pool.QueryRow(ctx, `
		SELECT o.name, o.website, o.portfolio_page, o.looked_up_at, s.id, s.name
		FROM organisations o
		JOIN LATERAL (SELECT s.id, s.name FROM sightings si JOIN sources s ON s.id = si.source_id
			WHERE si.organisation_id = o.id AND s.kind = 'portfolio' ORDER BY si.id LIMIT 1) s ON true
		WHERE o.id = $1`, orgID).Scan(&rec.name, &rec.url, &portfolioPage, &lookedUp, &rec.sourceID, &rec.sourceName)
	recent := lookedUp != nil && !lookedUp.Before(p.relookupBefore(cfg))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (recent || rec.url == "" && portfolioPage == "")) {
		return nil
	}
	if err != nil {
		return err
	}

	// A startup known by its page on the portfolio's site gets its website
	// from there.
	if rec.url == "" {
		rec.url = portfolioPage
		about, err := p.page(ctx, portfolioPage)
		if err != nil {
			rec.err = err.Error()
			rec.lookedUp = blocked(err, about)
			return p.finishLookUp(ctx, rec, extract.Imprint{})
		}
		rec.website = extract.StartupWebsite(about.Body, portfolioPage)
		if rec.website == "" {
			rec.lookedUp, rec.note = true, "no website on its portfolio page"
			return p.finishLookUp(ctx, rec, extract.Imprint{})
		}
		rec.url = rec.website
	}

	home, err := p.page(ctx, rec.url)
	if err != nil {
		rec.err = err.Error()
		// A site that refuses the bot is not asked again.
		rec.lookedUp = blocked(err, home)
		if !rec.lookedUp {
			// The third time a site does not answer, the startup counts as
			// gone, until a later lookup finds it again.
			var failed int
			if err := p.Pool.QueryRow(ctx, `
				SELECT count(*) FROM startup_lookups WHERE organisation_id = $1 AND error <> '' AND looked_up_at > COALESCE($2::timestamptz, '-infinity')`,
				orgID, lookedUp).Scan(&failed); err != nil {
				return err
			}
			if failed+1 >= 3 {
				rec.lookedUp, rec.activity, rec.activityNote = true, "gone", "the website did not answer three times"
			}
		}
		return p.finishLookUp(ctx, rec, extract.Imprint{})
	}
	base := home.FinalURL
	if base == "" {
		base = rec.url
	}
	if extract.Parked(home.Text) {
		rec.lookedUp, rec.note = true, "the website is a parked domain"
		rec.activity, rec.activityNote = "gone", "the website is a parked domain"
		return p.finishLookUp(ctx, rec, extract.Imprint{})
	}
	p.signsOfLife(ctx, &rec, cfg, home, base, "")
	rec.imprint = extract.ImprintLink(home.Body, base)
	if rec.imprint == "" {
		rec.imprint = strings.TrimSuffix(base, "/") + "/impressum"
	}
	imp, err := p.page(ctx, rec.imprint)
	rec.lookedUp = true
	if err != nil {
		rec.note = "no imprint found"
		rec.imprint = ""
		return p.finishLookUp(ctx, rec, extract.Imprint{})
	}
	im := extract.ParseImprint(imp.Text)
	if extract.InLiquidation(imp.Text) {
		rec.activity, rec.activityNote = "dissolved", "the imprint says the company is being wound up"
	}
	// The sitemap says when the site last changed. It is only worth a
	// request when the imprint named founders.
	if im.Young() && rec.activity != "dissolved" {
		if sm, err := p.Fetcher.HTTP(ctx, strings.TrimSuffix(base, "/")+"/sitemap.xml"); err == nil {
			p.signsOfLife(ctx, &rec, cfg, home, base, sm.Body)
		}
	}
	rec.profiles = append(extract.ProfileLinks(home.Body, base), extract.ProfileLinks(imp.Body, rec.imprint)...)
	// The team page often links each founder's profile. It is only worth a
	// request when the imprint named founders.
	if team := extract.TeamLink(home.Body, base); im.Young() && team != "" && team != rec.imprint {
		if tp, err := p.page(ctx, team); err == nil {
			rec.profiles = append(rec.profiles, extract.ProfileLinks(tp.Body, team)...)
		} else {
			p.log().Info("a team page failed", "url", team, "error", err)
		}
	}
	return p.finishLookUp(ctx, rec, im)
}

// signsOfLife sets how active the startup looks from the newest date its
// site shows: the sitemap's newest change or the copyright year. Without
// any date it is unknown. Being wound up or gone is set elsewhere and wins.
func (p *Pipeline) signsOfLife(_ context.Context, rec *lookUpRecord, cfg settings.Settings, home *fetch.Page, base, sitemap string) {
	if rec.activity == "dissolved" || rec.activity == "gone" {
		return
	}
	now := p.now()
	var newest time.Time
	var from string
	if t := extract.SitemapNewest(sitemap, now); !t.IsZero() {
		newest, from = t, "its sitemap"
	}
	if y := extract.CopyrightYear(home.Text); y > 0 {
		// A copyright of this year is today, an older one the year's end.
		t := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		if y >= now.Year() {
			t = now
		}
		if t.After(newest) {
			newest, from = t, "its copyright"
		}
	}
	if newest.IsZero() {
		rec.activity, rec.activityNote, rec.lastSign = "unknown", "the website shows no date", nil
		return
	}
	rec.lastSign = &newest
	rec.activity = "active"
	if now.Sub(newest) > cfg.Days("quiet_after_days", 365) {
		rec.activity = "quiet"
	}
	rec.activityNote = "the website changed " + newest.Format("January 2006") + ", by " + from
}

// page loads a page, with the browser when it is an empty JavaScript shell.
func (p *Pipeline) page(ctx context.Context, url string) (*fetch.Page, error) {
	page, err := p.Fetcher.HTTP(ctx, url)
	if err == nil && page.IsHTML() && fetch.LooksLikeJSShell(page) && p.Fetcher.HasBrowser() {
		return p.Fetcher.Browser(ctx, url)
	}
	return page, err
}

// finishLookUp stores what the imprint says in one short transaction.
func (p *Pipeline) finishLookUp(ctx context.Context, rec lookUpRecord, im extract.Imprint) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if rec.lookedUp {
		if _, err := tx.Exec(ctx, `
			UPDATE organisations SET looked_up_at = $2, imprint_url = $3,
				name = CASE WHEN $4 <> '' THEN $4 ELSE name END,
				normalised_name = CASE WHEN $5 <> '' THEN $5 ELSE normalised_name END,
				city = CASE WHEN city = '' THEN $6 ELSE city END,
				website = CASE WHEN website = '' THEN $7 ELSE website END,
				website_domain = CASE WHEN website_domain = '' THEN $8 ELSE website_domain END,
				activity = CASE WHEN $9 <> '' THEN $9 ELSE activity END,
				last_sign_at = CASE WHEN $9 <> '' THEN $10 ELSE last_sign_at END,
				activity_note = CASE WHEN $9 <> '' THEN $11 ELSE activity_note END,
				updated_at = $2
			WHERE id = $1`, rec.orgID, rec.at, rec.imprint, im.Company, NormaliseOrg(im.Company), im.City, rec.website, domainOf(rec.website),
			rec.activity, rec.lastSign, rec.activityNote); err != nil {
			return err
		}
	}
	company := im.Company
	if company == "" {
		company = rec.name
	}
	switch {
	case rec.err != "" || rec.note != "":
	case len(im.Directors) == 0:
		rec.note = "the imprint names nobody"
	case !im.Young():
		rec.note = "not a young company"
		if im.LegalForm != "" {
			rec.note += " (" + im.LegalForm + ")"
		}
	default:
		label := strings.ToUpper(im.Label[:1]) + im.Label[1:]
		evidence := fmt.Sprintf("%s of %s, by its imprint. In the portfolio of %s", label, company, rec.sourceName)
		for _, name := range im.Directors {
			isNew, err := p.resolveFounder(ctx, tx, rec, name, im.City, im.Label+", "+company, evidence)
			if err != nil {
				return err
			}
			rec.found++
			if isNew {
				rec.isNew++
			}
		}
	}
	rec.duration = p.now().Sub(rec.at)
	if _, err := tx.Exec(ctx, `
		INSERT INTO startup_lookups (run_id, organisation_id, url, imprint_url, people_found, people_new, note, error, duration_ms, looked_up_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		nullID(rec.runID), rec.orgID, rec.url, rec.imprint, rec.found, rec.isNew, rec.note, rec.err, rec.duration.Milliseconds(), rec.at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// resolveFounder finds the person among those who run this startup, or
// adds them, and marks them a founder with the imprint as evidence.
func (p *Pipeline) resolveFounder(ctx context.Context, tx pgx.Tx, rec lookUpRecord, name, city, headline, evidence string) (bool, error) {
	norm := extract.NormaliseName(name)
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT p.id FROM people p JOIN affiliations af ON af.person_id = p.id
		WHERE p.normalised_name = $1 AND af.organisation_id = $2 ORDER BY p.id LIMIT 1`, norm, rec.orgID).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	isNew := id == 0
	if isNew {
		err = tx.QueryRow(ctx, `
			INSERT INTO people (full_name, normalised_name, city, headline, fit, fit_evidence, created_at, updated_at, status_changed_at)
			VALUES ($1, $2, $3, $4, 'founder', $5, $6, $6, $6) RETURNING id`,
			name, norm, city, headline, evidence, rec.at).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE people SET fit = 'founder', updated_at = $3,
				fit_evidence = CASE WHEN fit_evidence = '' THEN $2 ELSE fit_evidence END
			WHERE id = $1`, id, evidence, rec.at)
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO affiliations (person_id, organisation_id, role) VALUES ($1, $2, 'founder') ON CONFLICT DO NOTHING`, id, rec.orgID); err != nil {
		return false, err
	}
	// Profiles the startup's own site links under this person's name. The
	// engine never opens them. Tim confirms or rejects each.
	for _, l := range extract.ProfilesOf(rec.profiles, name) {
		platform, clean := ProfileOf(l)
		if clean == "" {
			continue
		}
		// Xing and GitHub have no platform of their own in the data model.
		if platform == "website" {
			platform = "other"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO profiles (person_id, platform, url, handle, found_via, source_id, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
			ON CONFLICT (platform, url) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at`,
			id, platform, clean, handleOf(clean), "website of "+strings.TrimSuffix(domainOf(rec.url), "/"), rec.sourceID, rec.at); err != nil {
			return false, err
		}
	}
	return isNew, sight(ctx, tx, rec.sourceID, rec.at, "person_id", id, isNew)
}

func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
