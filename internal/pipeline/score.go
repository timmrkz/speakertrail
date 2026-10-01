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
// on stage, what they founded as lookups saw it, and what pages that are
// not kept said about them, like an about page or the model's read.
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
			FROM page_signals m WHERE m.person_id = p.id)
	FROM people p WHERE p.id = ANY($1)`

// ScorePeople runs the rubric over the given people and stores their
// signals and score, in one short transaction.
func ScorePeople(ctx context.Context, pool *pgxpool.Pool, ids []int64) error {
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
		var in rubric.Person
		var apps, companies, found []byte
		if err := rows.Scan(&id, &in.Headline, &in.FitEvidence, &apps, &companies, &found); err != nil {
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
		all = append(all, scored{id, rubric.Signals(in)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
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

// ScoreStale scores everyone the current rubric has not scored yet, a
// batch at a time, and returns how many it scored.
func ScoreStale(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	n := 0
	for {
		rows, err := pool.Query(ctx, `SELECT id FROM people WHERE rubric <> $1 ORDER BY id LIMIT $2`, rubric.Version, scoreBatch)
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
		if err := ScorePeople(ctx, pool, ids); err != nil {
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
	if err := ScorePeople(ctx, p.Pool, ids); err != nil {
		p.log().Warn("scoring people failed", "people", len(ids), "error", err)
		if _, err := p.Pool.Exec(context.WithoutCancel(ctx), `UPDATE people SET rubric = 0 WHERE id = ANY($1)`, ids); err != nil {
			p.log().Warn("marking people for scoring failed", "error", err)
		}
	}
}
