package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// scoreInput loads what the rubric reads about each person: their title,
// the founder passage, their appearances with the passages that put them
// on stage, what they founded as lookups saw it, what pages that are not
// kept said about them, like an about page or the model's read, their
// newest post on LinkedIn, and how old their confirmed LinkedIn profile
// says they likely are.
const scoreInput = `
	SELECT p.id, p.headline, p.fit_evidence,
		(SELECT COALESCE(json_agg(json_build_object('Role', a.role, 'Event', e.title, 'Evidence', a.evidence) ORDER BY e.starts_at, e.id), '[]')
			FROM appearances a JOIN events e ON e.id = a.event_id WHERE a.person_id = p.id),
		(SELECT COALESCE(json_agg(json_build_object('Name', o.name, 'Activity', o.activity, 'ActivityNote', o.activity_note,
				'Founders', (SELECT count(*) FROM affiliations x WHERE x.organisation_id = o.id AND x.role = 'founder'),
				'Portfolio', COALESCE((SELECT s.name FROM sightings si JOIN sources s ON s.id = si.source_id
					WHERE si.organisation_id = o.id AND s.kind = 'portfolio' ORDER BY si.id LIMIT 1), ''),
				'Note', COALESCE((SELECT l.note FROM startup_lookups l WHERE l.organisation_id = o.id ORDER BY l.id DESC LIMIT 1), ''))
				ORDER BY o.id), '[]')
			FROM affiliations af JOIN organisations o ON o.id = af.organisation_id WHERE af.person_id = p.id AND af.role = 'founder'),
		(SELECT COALESCE(json_agg(json_build_object('Key', m.signal, 'Passage', m.passage, 'Where', m.found_in) ORDER BY m.found_at, m.signal), '[]')
			FROM page_signals m WHERE m.person_id = p.id),
		(SELECT json_build_object('Title', pp.title, 'Published', pp.published_at)
			FROM person_posts pp WHERE pp.person_id = p.id AND pp.published_at IS NOT NULL
			ORDER BY pp.published_at DESC, pp.id DESC LIMIT 1),
		(SELECT json_build_object('BornAbout', pr.born_about, 'AgeReason', pr.age_reason)
			FROM profiles pr WHERE pr.person_id = p.id AND pr.platform = 'linkedin' AND pr.review = 'confirmed' AND pr.born_about IS NOT NULL
			ORDER BY pr.last_seen_at DESC, pr.id DESC LIMIT 1)
	FROM people p WHERE p.id = ANY($1)`

// ScorePeople runs the rubric over the given people as of now and stores
// their signals and score, in one short transaction.
func ScorePeople(ctx context.Context, pool *pgxpool.Pool, ids []int64, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := pool.Query(ctx, scoreInput, ids)
	if err != nil {
		return err
	}
	type scored struct {
		id      int64
		signals []rubric.Signal
	}
	var all []scored
	for rows.Next() {
		var id int64
		in := rubric.Person{Now: now}
		var apps, companies, found, post, age []byte
		if err := rows.Scan(&id, &in.Headline, &in.FitEvidence, &apps, &companies, &found, &post, &age); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(apps, &in.Appearances); err != nil {
			rows.Close()
			return fmt.Errorf("appearances of %d: %w", id, err)
		}
		if err := json.Unmarshal(companies, &in.Companies); err != nil {
			rows.Close()
			return fmt.Errorf("companies of %d: %w", id, err)
		}
		if err := json.Unmarshal(found, &in.Found); err != nil {
			rows.Close()
			return fmt.Errorf("page signals of %d: %w", id, err)
		}
		if post != nil {
			if err := json.Unmarshal(post, &in.LastPost); err != nil {
				rows.Close()
				return fmt.Errorf("last post of %d: %w", id, err)
			}
		}
		if age != nil {
			if err := json.Unmarshal(age, &in); err != nil {
				rows.Close()
				return fmt.Errorf("age of %d: %w", id, err)
			}
		}
		all = append(all, scored{id, rubric.Signals(in)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		// Checks side by side score the same person. The second waits here
		// for the first, then replaces its signals, rather than both adding
		// the same ones.
		if _, err := tx.Exec(ctx, `SELECT id FROM people WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM person_signals WHERE person_id = ANY($1)`, ids); err != nil {
			return err
		}
		for _, s := range all {
			for _, sig := range s.signals {
				if _, err := tx.Exec(ctx, `INSERT INTO person_signals (person_id, signal, is_for, label, passage, found_in) VALUES ($1, $2, $3, $4, $5, $6)`,
					s.id, sig.Key, sig.For, rubric.Lookup(sig.Key).Label, sig.Passage, sig.Where); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE people SET fit_score = $2, rubric = $3 WHERE id = $1`, s.id, rubric.Score(s.signals), rubric.Version); err != nil {
				return err
			}
		}
		return nil
	})
}

// pageSignal keeps a signal found on a page that is not kept, for the
// rubric, once per passage.
func pageSignal(ctx context.Context, db execer, personID int64, signal, passage, where string, at time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO page_signals (person_id, signal, passage, found_in, found_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`, personID, signal, passage, where, at)
	return err
}

// scoreBatch is how many people one transaction scores.
const scoreBatch = 200

// staleQuery finds people the current rubric has not scored yet, people
// who still write on LinkedIn by their score while their newest post has
// grown older than a year since, and people under 30 by their score who
// have turned 30 since.
const staleQuery = `
	SELECT id FROM people p WHERE rubric <> $1
	   OR (EXISTS (SELECT 1 FROM person_signals s WHERE s.person_id = p.id AND s.signal = 'writes_on_linkedin')
	       AND NOT EXISTS (SELECT 1 FROM person_posts pp WHERE pp.person_id = p.id AND pp.published_at >= $3))
	   OR (EXISTS (SELECT 1 FROM person_signals s WHERE s.person_id = p.id AND s.signal = 'young')
	       AND NOT EXISTS (SELECT 1 FROM profiles pr WHERE pr.person_id = p.id AND pr.platform = 'linkedin' AND pr.review = 'confirmed'
	           AND pr.born_about > $4::int - $5::int))
	ORDER BY id LIMIT $2`

// ScoreStale scores everyone whose score is out of date as of now, a batch
// at a time, and returns how many it scored.
func ScoreStale(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int, error) {
	n := 0
	for {
		rows, err := pool.Query(ctx, staleQuery, rubric.Version, scoreBatch, now.Add(-rubric.QuietAfter), now.Year(), rubric.YoungUntil)
		if err != nil {
			return n, err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return n, err
		}
		if len(ids) == 0 {
			return n, nil
		}
		if err := ScorePeople(ctx, pool, ids, now); err != nil {
			return n, err
		}
		n += len(ids)
	}
}

// score scores people the pipeline just touched. A failure is logged, not
// returned: the person keeps their old score, and the next run scores
// them again.
func (p *Pipeline) score(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	if err := ScorePeople(ctx, p.Pool, ids, p.now()); err != nil {
		p.log().Warn("scoring people failed", "people", len(ids), "error", err)
		if _, err := p.Pool.Exec(context.WithoutCancel(ctx), `UPDATE people SET rubric = 0 WHERE id = ANY($1)`, ids); err != nil {
			p.log().Warn("marking people for scoring failed", "error", err)
		}
	}
}
