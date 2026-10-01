package server

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// decide keeps or skips a person, or undoes it with an empty decision, and
// records the decision with the signals the person has now. It reports
// whether the person exists.
func (s *Server) decide(ctx context.Context, id int64, decision string) (bool, error) {
	status := map[string]string{"": "new", "kept": "kept", "skipped": "skipped"}[decision]
	found := false
	err := pgx.BeginFunc(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE people SET podcast_status = $2, status_changed_at = now(), updated_at = now() WHERE id = $1`, id, status)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		found = true
		if decision == "" {
			_, err = tx.Exec(ctx, `DELETE FROM fit_decisions WHERE person_id = $1`, id)
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO fit_decisions (person_id, decision, signals, fit_score, rubric, decided_at)
			SELECT p.id, $2, COALESCE((SELECT array_agg(signal ORDER BY signal) FROM person_signals WHERE person_id = p.id), '{}'),
				p.fit_score, p.rubric, now()
			FROM people p WHERE p.id = $1
			ON CONFLICT (person_id) WHERE person_id IS NOT NULL DO UPDATE SET
				decision = EXCLUDED.decision, signals = EXCLUDED.signals, fit_score = EXCLUDED.fit_score,
				rubric = EXCLUDED.rubric, decided_at = EXCLUDED.decided_at`, id, decision)
		return err
	})
	return found, err
}

// topByFit is how many of the best fits the brief measures: at least half
// of the top 20 should be people Tim would contact.
const topByFit = 20

// fitJSON says what Tim's decisions teach the rubric: for every signal, in
// the rubric's order, how often a person with it was kept or skipped, and
// how the top 20 by fit were decided.
var fitJSON = func() string {
	var rows []string
	for i, d := range rubric.Defs {
		rows = append(rows, "("+strconv.Itoa(i)+", '"+d.Key+"', '"+strings.ReplaceAll(d.Label, "'", "''")+"', "+map[bool]string{true: "true", false: "false"}[d.For]+")")
	}
	return `json_build_object(
		'signals', (SELECT json_agg(json_build_object('key', d.key, 'label', d.label, 'for', d.is_for,
				'kept', (SELECT count(*) FROM fit_decisions f WHERE f.decision = 'kept' AND d.key = ANY(f.signals)),
				'skipped', (SELECT count(*) FROM fit_decisions f WHERE f.decision = 'skipped' AND d.key = ANY(f.signals)))
				ORDER BY d.n)
			FROM (VALUES ` + strings.Join(rows, ", ") + `) AS d(n, key, label, is_for)),
		'top', (SELECT json_build_object('size', count(*),
				'kept', count(*) FILTER (WHERE t.podcast_status NOT IN ('new', 'known', 'skipped')),
				'skipped', count(*) FILTER (WHERE t.podcast_status = 'skipped'),
				'open', count(*) FILTER (WHERE t.podcast_status IN ('new', 'known')))
			FROM (SELECT podcast_status FROM people ORDER BY fit_score DESC, created_at DESC, id DESC LIMIT ` + strconv.Itoa(topByFit) + `) t))`
}()
