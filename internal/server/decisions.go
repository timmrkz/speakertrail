package server

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// connect marks a person as connected, or open again, and records it
// with the signals the person has now. Tim connects with someone on
// LinkedIn. Whoever he does not connect with stays open, there is no
// skipping. A person further along the funnel, contacted or recorded,
// stays where they are when marked connected again. It reports whether the
// person exists.
func (s *Server) connect(ctx context.Context, id int64, connected bool) (bool, error) {
	found := false
	err := pgx.BeginFunc(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		set := `CASE WHEN podcast_status IN ('new', 'known') THEN 'connected' ELSE podcast_status END`
		if !connected {
			set = `'new'`
		}
		tag, err := tx.Exec(ctx, `UPDATE people SET podcast_status = `+set+`, status_changed_at = now(), updated_at = now() WHERE id = $1`, id)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		found = true
		if !connected {
			_, err = tx.Exec(ctx, `DELETE FROM fit_decisions WHERE person_id = $1`, id)
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO fit_decisions (person_id, decision, signals, fit_score, rubric, decided_at)
			SELECT p.id, 'connected', COALESCE((SELECT array_agg(signal ORDER BY signal) FROM person_signals WHERE person_id = p.id), '{}'),
				p.fit_score, p.rubric, now()
			FROM people p WHERE p.id = $1
			ON CONFLICT (person_id) WHERE person_id IS NOT NULL DO UPDATE SET
				signals = EXCLUDED.signals, fit_score = EXCLUDED.fit_score,
				rubric = EXCLUDED.rubric, decided_at = EXCLUDED.decided_at`, id)
		return err
	})
	return found, err
}

// isConnected is the SQL that tells whether the person p is connected:
// marked connected, or further along the funnel.
const isConnected = `p.podcast_status NOT IN ('new', 'known')`

// topByFit is how many of the best fits the brief measures: at least half
// of the top 20 should be people Tim would contact.
const topByFit = 20

// fitJSON says what Tim's connections teach the rubric: for every signal,
// in the rubric's order, how many people with it he connected with, and
// how many of the top 20 by fit he connected with.
var fitJSON = func() string {
	var rows []string
	for i, d := range rubric.Defs {
		rows = append(rows, "("+strconv.Itoa(i)+", '"+d.Key+"', '"+strings.ReplaceAll(d.Label, "'", "''")+"', "+map[bool]string{true: "true", false: "false"}[d.For]+")")
	}
	return `json_build_object(
		'signals', (SELECT json_agg(json_build_object('key', d.key, 'label', d.label, 'for', d.is_for,
				'connected', (SELECT count(*) FROM fit_decisions f WHERE d.key = ANY(f.signals)))
				ORDER BY d.n)
			FROM (VALUES ` + strings.Join(rows, ", ") + `) AS d(n, key, label, is_for)),
		'top', (SELECT json_build_object('size', count(*),
				'connected', count(*) FILTER (WHERE ` + isConnected + `),
				'open', count(*) FILTER (WHERE NOT ` + isConnected + `))
			FROM (SELECT podcast_status FROM people p ORDER BY fit_score DESC, created_at DESC, id DESC LIMIT ` + strconv.Itoa(topByFit) + `) p))`
}()
