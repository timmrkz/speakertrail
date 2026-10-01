package server

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/pipeline"
)

// failure is what went wrong in a run, in plain words: one per source for
// checks, and one per reason for reads and lookups, with the one action
// that fixes it. Action is "retire" to retire the source, "check" to check
// it again, or empty when the engine already did what it could, or when
// nothing on the page can fix it.
type failure struct {
	What   string         `json:"what"`
	Source *failureSource `json:"source"`
	Reason string         `json:"reason"`
	Action string         `json:"action"`
	Count  int            `json:"count"`
	// Detail is the error as recorded, for a closer look.
	Detail string `json:"detail"`
}

type failureSource struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

// failuresOf collects what failed in a run.
func (s *Server) failuresOf(ctx context.Context, runID int64) ([]failure, error) {
	out := []failure{}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT s.id, s.name, COALESCE(s.url, ''), s.status, c.error, c.http_status,
			-- The source failed its last three checks, not only this one.
			(SELECT count(*) FROM (SELECT x.error FROM source_checks x WHERE x.source_id = s.id
				ORDER BY x.checked_at DESC, x.id DESC LIMIT 3) l WHERE l.error <> '') = 3
		FROM source_checks c JOIN sources s ON s.id = c.source_id
		WHERE c.run_id = $1 AND c.error <> ''
		ORDER BY s.name, s.id, c.id`, runID)
	if err != nil {
		return nil, err
	}
	seen := map[int64]int{}
	for rows.Next() {
		var src failureSource
		var msg string
		var status int
		var inRow bool
		if err := rows.Scan(&src.ID, &src.Name, &src.URL, &src.Status, &msg, &status, &inRow); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := seen[src.ID]; ok {
			out[i].Count++
			continue
		}
		reason, kind := pipeline.Explain(msg, status)
		action := ""
		switch {
		case src.Status == "retired" || src.Status == "manual" || kind == pipeline.FailEngine:
		case kind == pipeline.FailPassing && inRow:
			reason += ", three checks in a row"
			action = "retire"
		case kind == pipeline.FailPassing:
			action = "check"
		default:
			action = "retire"
		}
		seen[src.ID] = len(out)
		out = append(out, failure{What: "check", Source: &src, Reason: reason, Action: action, Count: 1, Detail: msg})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, what := range []struct{ name, sql string }{
		{"read", `SELECT error FROM event_reads WHERE run_id = $1 AND error <> '' ORDER BY id`},
		{"lookup", `SELECT error FROM startup_lookups WHERE run_id = $1 AND error <> '' ORDER BY id`},
	} {
		rows, err := s.opts.Pool.Query(ctx, what.sql, runID)
		if err != nil {
			return nil, err
		}
		msgs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return nil, err
		}
		var group []failure
		byReason := map[string]int{}
		for _, msg := range msgs {
			reason, _ := pipeline.Explain(msg, 0)
			if i, ok := byReason[reason]; ok {
				group[i].Count++
				continue
			}
			byReason[reason] = len(group)
			group = append(group, failure{What: what.name, Reason: reason, Count: 1, Detail: msg})
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].Count > group[j].Count })
		out = append(out, group...)
	}
	return out, nil
}
