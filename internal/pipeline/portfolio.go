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

// finishPortfolio stores the startups a portfolio lists, one short
// transaction, and queues lookups of the ones not looked up yet.
func (p *Pipeline) finishPortfolio(ctx context.Context, src Source, runID int64, cfg settings.Settings, page *fetch.Page, rec checkRecord) error {
	base := page.FinalURL
	if base == "" {
		base = src.URL
	}
	startups := extract.StartupLinks(page.Body, base)
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
		err := tx.QueryRow(ctx, `SELECT id FROM organisations WHERE website_domain = $1 ORDER BY id LIMIT 1`, domainOf(s.Website)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			id, isNew, err = upsertOrganisation(ctx, tx, s.Name, "company", s.Website, "")
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
// up yet. One that failed three times is given up.
const unlookedStartups = `
	SELECT o.id FROM organisations o
	WHERE o.looked_up_at IS NULL AND o.website <> ''
	  AND EXISTS (SELECT 1 FROM sightings si JOIN sources s ON s.id = si.source_id
		WHERE si.organisation_id = o.id AND s.kind = 'portfolio' AND ($2 = 0 OR s.id = $2))
	  AND (SELECT count(*) FROM startup_lookups l WHERE l.organisation_id = o.id AND l.error <> '') < 3
	ORDER BY o.id LIMIT $1`

// enqueueLookUps queues lookups for a run. With a source it only takes
// startups that portfolio lists.
func (p *Pipeline) enqueueLookUps(ctx context.Context, runID, sourceID int64, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := p.Pool.Query(ctx, unlookedStartups, limit, sourceID)
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
	runID, orgID      int64
	url, imprint      string
	note, err         string
	duration          time.Duration
	at                time.Time
	found, isNew      int
	sourceID          int64
	sourceName, name  string
	lookedUp, renamed bool
}

// LookUp loads a startup's website, finds its imprint and stores the
// people it names as the ones who run the company. A site that cannot be
// reached is tried again by a later run, three times at most.
func (p *Pipeline) LookUp(ctx context.Context, orgID, runID int64) error {
	rec := lookUpRecord{runID: runID, orgID: orgID, at: p.now()}
	var lookedUp *time.Time
	err := p.Pool.QueryRow(ctx, `
		SELECT o.name, o.website, o.looked_up_at, s.id, s.name
		FROM organisations o
		JOIN LATERAL (SELECT s.id, s.name FROM sightings si JOIN sources s ON s.id = si.source_id
			WHERE si.organisation_id = o.id AND s.kind = 'portfolio' ORDER BY si.id LIMIT 1) s ON true
		WHERE o.id = $1`, orgID).Scan(&rec.name, &rec.url, &lookedUp, &rec.sourceID, &rec.sourceName)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (lookedUp != nil || rec.url == "")) {
		return nil
	}
	if err != nil {
		return err
	}

	home, err := p.page(ctx, rec.url)
	if err != nil {
		rec.err = err.Error()
		// A site that refuses the bot is not asked again.
		rec.lookedUp = blocked(err, home)
		return p.finishLookUp(ctx, rec, extract.Imprint{})
	}
	base := home.FinalURL
	if base == "" {
		base = rec.url
	}
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
	return p.finishLookUp(ctx, rec, extract.ParseImprint(imp.Text))
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
				updated_at = $2
			WHERE id = $1`, rec.orgID, rec.at, rec.imprint, im.Company, NormaliseOrg(im.Company), im.City); err != nil {
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
	return isNew, sight(ctx, tx, rec.sourceID, rec.at, "person_id", id, isNew)
}

func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
