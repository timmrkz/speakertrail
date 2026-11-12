package pipeline

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/rubric"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// checkProfileSearch runs a profile search: Exa finds LinkedIn profiles
// of a kind of person in a place, like "Personal Trainer Düsseldorf", from
// its own index, in one call. The engine never opens LinkedIn. A person is
// kept only when they live in NRW, were active on LinkedIn lately, run
// their own business, and nothing rules them out, like consulting or one
// of Tim's exclusions. Nothing is stored of anyone else. Without Exa, or with its budget spent, the
// profile search waits.
func (p *Pipeline) checkProfileSearch(ctx context.Context, src Source, runID int64) error {
	if p.Search == nil || !p.Search.FindsPosts() || src.Query == "" || src.Status == "manual" {
		return nil
	}
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	excluded, err := Exclusions(ctx, p.Pool)
	if err != nil {
		return err
	}
	start := p.now()
	rec := checkRecord{checkedAt: start, mode: "search"}
	profiles, _, err := p.Search.People(ctx, src.Query, cfg.Int("profile_search_results", 25))
	if errors.Is(err, search.ErrNoBudget) {
		p.log().Info("a profile search waits, no searches left this month", "source", src.ID)
		return nil
	}
	if err != nil {
		return p.searchFailed(ctx, src, runID, cfg, rec, err)
	}

	activeSince := start.Add(-cfg.Days("profile_active_days", 183))
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	seen := map[string]bool{}
	for _, prof := range profiles {
		_, key := ProfileOf(prof.URL)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if !guest(prof, start, activeSince, excluded) {
			continue
		}
		city, _ := extract.PlaceInNRW(prof.Location)
		id, isNew, err := p.resolveProfile(ctx, tx, src, runID, key, prof, city)
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

// guest tells whether a profile shows a possible guest: in NRW, active on
// LinkedIn since a time, running their own business, and neither ruled out
// nor left out by one of the exclusions.
func guest(prof search.Profile, now, activeSince time.Time, exclusions []string) bool {
	if _, inNRW := extract.PlaceInNRW(prof.Location); !inNRW {
		return false
	}
	if prof.LastActive.Before(activeSince) {
		return false
	}
	_, _, roles := career(prof.Career)
	if _, out := rubric.Excluded(prof.Headline+" "+roles, exclusions); out {
		return false
	}
	ok, _ := rubric.Guest(rubric.Signals(rubric.Person{Name: prof.Name, Headline: prof.Headline, CurrentRoles: roles, Now: now}))
	return ok
}

// resolveProfile finds the person behind a LinkedIn profile, or adds them,
// with what is kept of the profile. The profile is theirs for certain,
// because it is how they were found, so it is confirmed.
func (p *Pipeline) resolveProfile(ctx context.Context, tx pgx.Tx, src Source, runID int64, profile string, prof search.Profile, city string) (int64, bool, error) {
	at := p.now()
	// Searches run side by side. Two finding the same person wait for each
	// other here, so the second finds the person the first added.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('profile:' || $1))`, profile); err != nil {
		return 0, false, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT person_id FROM profiles WHERE platform = 'linkedin' AND url = $1 AND person_id IS NOT NULL`, profile).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	evidence := "Found by the profile search " + src.Name
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
				headline = CASE WHEN $4 <> '' THEN $4 ELSE headline END
			WHERE id = $1`, id, at, city, prof.Headline)
	}
	if err != nil {
		return 0, false, err
	}
	bornAbout, ageReason, roles := careerOf(prof, at)
	var lastActive *time.Time
	if !prof.LastActive.IsZero() {
		lastActive = &prof.LastActive
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO profiles (person_id, platform, url, handle, headline, review, found_via, source_id, first_seen_at, last_seen_at,
			born_about, age_reason, current_roles, last_active_at)
		VALUES ($1, 'linkedin', $2, $3, $4, 'confirmed', 'profile search', $5, $6, $6, $7, $8, $9, $10)
		ON CONFLICT (platform, url) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, review = 'confirmed',
			headline = CASE WHEN EXCLUDED.headline <> '' THEN EXCLUDED.headline ELSE profiles.headline END,
			born_about = COALESCE(EXCLUDED.born_about, profiles.born_about),
			age_reason = CASE WHEN EXCLUDED.born_about IS NOT NULL THEN EXCLUDED.age_reason ELSE profiles.age_reason END,
			current_roles = CASE WHEN EXCLUDED.current_roles <> '' THEN EXCLUDED.current_roles ELSE profiles.current_roles END,
			last_active_at = GREATEST(EXCLUDED.last_active_at, profiles.last_active_at)`,
		id, profile, handleOf(profile), prof.Headline, src.ID, at, bornAbout, ageReason, roles, lastActive); err != nil {
		return 0, false, err
	}
	return id, isNew, sight(ctx, tx, src.ID, at, "person_id", id, isNew)
}
