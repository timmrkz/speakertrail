package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// checkPostSearch runs a post search: Exa finds LinkedIn posts of the last
// months, each post's link names its author, and the authors' profiles
// are read from Exa's index, all of them in one call. Only authors who
// live in NRW are kept, each with the post that brought them. The engine
// never opens LinkedIn. Without a provider that finds posts, or with its
// budget spent, the post search waits.
func (p *Pipeline) checkPostSearch(ctx context.Context, src Source, runID int64) error {
	if p.Search == nil || !p.Search.FindsPosts() || src.Query == "" || src.Status == "manual" {
		return nil
	}
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	start := p.now()
	rec := checkRecord{checkedAt: start, mode: "search"}
	posts, _, err := p.Search.Posts(ctx, src.Query, start.Add(-cfg.Days("post_search_days", 90)))
	if errors.Is(err, search.ErrNoBudget) {
		p.log().Info("a post search waits, no searches left this month", "source", src.ID)
		return nil
	}
	if err != nil {
		return p.searchFailed(ctx, src, runID, cfg, rec, err)
	}

	// One post per author, the newest.
	byAuthor := map[string]search.Post{}
	var authors []string
	for _, post := range posts {
		_, key := ProfileOf(post.Author)
		if key == "" {
			continue
		}
		if old, ok := byAuthor[key]; ok {
			if post.Published.After(old.Published) {
				byAuthor[key] = post
			}
			continue
		}
		byAuthor[key] = post
		authors = append(authors, post.Author)
	}
	profiles, _, err := p.Search.Profiles(ctx, authors)
	if errors.Is(err, search.ErrNoBudget) {
		p.log().Info("a post search waits, no lookups left this month", "source", src.ID)
		return nil
	}
	if err != nil {
		return p.searchFailed(ctx, src, runID, cfg, rec, err)
	}

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	seen := map[string]bool{}
	for _, prof := range profiles {
		_, key := ProfileOf(prof.URL)
		post, ok := byAuthor[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		city, inNRW := extract.PlaceInNRW(prof.Location)
		if !inNRW {
			continue
		}
		id, isNew, err := p.resolvePoster(ctx, tx, src, runID, key, prof, city, post)
		if err != nil {
			return err
		}
		rec.stats.People = append(rec.stats.People, id)
		rec.stats.PeopleFound++
		if isNew {
			rec.stats.PeopleNew++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	rec.status = 200
	rec.duration = p.now().Sub(start)
	if err := p.recordCheck(ctx, src, runID, rec); err != nil {
		return err
	}
	p.score(ctx, rec.stats.People)
	return p.advancePortfolio(ctx, src, cfg, rec.stats.PeopleFound, "")
}

// searchFailed records a search whose provider failed. The provider
// failed, not the search, so it is tried again tomorrow.
func (p *Pipeline) searchFailed(ctx context.Context, src Source, runID int64, cfg settings.Settings, rec checkRecord, cause error) error {
	rec.err = cause.Error()
	rec.duration = p.now().Sub(rec.checkedAt)
	if err := p.recordCheck(ctx, src, runID, rec); err != nil {
		return err
	}
	_, err := p.Pool.Exec(ctx, `UPDATE sources SET last_checked_at = $2, next_check_at = $3 WHERE id = $1`,
		src.ID, rec.checkedAt, rec.checkedAt.Add(cfg.Days("active_check_interval_days", 1)))
	return err
}

// resolvePoster finds the person behind a LinkedIn profile, or adds them.
// The profile is theirs for certain, because it is how they were found,
// so it is confirmed. The post that brought them is kept with its date.
func (p *Pipeline) resolvePoster(ctx context.Context, tx pgx.Tx, src Source, runID int64, profile string, prof search.Profile, city string, post search.Post) (int64, bool, error) {
	at := p.now()
	// Post searches run side by side. Two finding the same author wait for
	// each other here, so the second finds the person the first added.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('profile:' || $1))`, profile); err != nil {
		return 0, false, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT person_id FROM profiles WHERE platform = 'linkedin' AND url = $1 AND person_id IS NOT NULL`, profile).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	evidence := postEvidence(post, src.Name)
	isNew := id == 0
	if isNew {
		err = tx.QueryRow(ctx, `
			INSERT INTO people (full_name, normalised_name, city, headline, fit_evidence, first_run_id, created_at, updated_at, status_changed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7) RETURNING id`,
			prof.Name, extract.NormaliseName(prof.Name), city, prof.Headline, evidence, nullID(runID), at).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE people SET updated_at = $2,
				city = CASE WHEN city = '' THEN $3 ELSE city END,
				headline = CASE WHEN headline = '' THEN $4 ELSE headline END,
				fit_evidence = CASE WHEN fit_evidence = '' THEN $5 ELSE fit_evidence END
			WHERE id = $1`, id, at, city, prof.Headline, evidence)
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO profiles (person_id, platform, url, handle, headline, review, found_via, source_id, first_seen_at, last_seen_at)
		VALUES ($1, 'linkedin', $2, $3, $4, 'confirmed', 'post search', $5, $6, $6)
		ON CONFLICT (platform, url) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at,
			headline = CASE WHEN EXCLUDED.headline <> '' THEN EXCLUDED.headline ELSE profiles.headline END`,
		id, profile, handleOf(profile), prof.Headline, src.ID, at); err != nil {
		return 0, false, err
	}
	var published *time.Time
	if !post.Published.IsZero() {
		published = &post.Published
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO person_posts (person_id, url, title, published_at, source_id, found_at)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (url) DO NOTHING`,
		id, post.URL, post.Title, published, src.ID, at); err != nil {
		return 0, false, err
	}
	return id, isNew, sight(ctx, tx, src.ID, at, "person_id", id, isNew)
}

// postEvidence says why a person appears: the post they wrote, and the
// post search that found it.
func postEvidence(post search.Post, search string) string {
	title := strings.TrimSpace(post.Title)
	when := ""
	if !post.Published.IsZero() {
		when = " on " + post.Published.Format("2 January 2006")
	}
	if title == "" {
		return fmt.Sprintf("Wrote on LinkedIn%s. Found by the post search %s", when, search)
	}
	return fmt.Sprintf("Wrote on LinkedIn%s: “%s”. Found by the post search %s", when, title, search)
}
