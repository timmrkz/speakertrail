package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/config"
	"github.com/timmrkz/speakertrail/internal/db"
	"github.com/timmrkz/speakertrail/internal/llm"
)

// runReport writes what the engine did lately as Markdown, for Tim to
// attach to a chat with Claude. It names sources, pages and errors, but no
// people, so it can be shared.
func runReport(ctx context.Context, cfg config.Config) error {
	return withDB(ctx, cfg, func(pool *pgxpool.Pool) error {
		if err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		return writeReport(ctx, pool, os.Stdout, time.Now())
	})
}

func writeReport(ctx context.Context, pool *pgxpool.Pool, w io.Writer, now time.Time) error {
	fmt.Fprintf(w, "# Speaker Trail report\n\n%s, version %s\n\n", now.Format("2006-01-02 15:04 MST"), version)
	if m := llm.FromEnvIfSet(); m != nil {
		fmt.Fprintf(w, "Language model: %s at %s\n\n", m.Model, m.URL)
	} else {
		fmt.Fprint(w, "Language model: none set up, event pages are not read\n\n")
	}

	sections := []struct {
		title, sql string
	}{
		{"Totals", `
			SELECT 'sources ' || s.status, count(*)::text FROM sources s GROUP BY s.status
			UNION ALL SELECT 'events kept, upcoming', count(*)::text FROM events WHERE fit = 'kept' AND starts_at >= now()
			UNION ALL SELECT 'events waiting for a read', count(*)::text FROM events
				WHERE fit = 'kept' AND starts_at >= now() AND people_read_at IS NULL AND canonical_url <> ''
			UNION ALL SELECT 'people', count(*)::text FROM people
			UNION ALL SELECT 'people with a founder role', count(*)::text FROM people WHERE fit = 'founder'`},
		{"Last runs", `
			SELECT r.id || ' ' || r.kind || ', ' || to_char(r.started_at, 'YYYY-MM-DD HH24:MI'),
				COALESCE(to_char(r.finished_at - r.started_at, 'HH24:MI:SS'), 'not finished') || ', ' ||
				(SELECT count(*) FROM source_checks c WHERE c.run_id = r.id) || ' checks, ' ||
				(SELECT count(*) FROM source_checks c WHERE c.run_id = r.id AND c.error <> '') || ' failed, ' ||
				(SELECT count(*) FROM event_reads er WHERE er.run_id = r.id) || ' reads, ' ||
				(SELECT count(*) FROM event_reads er WHERE er.run_id = r.id AND er.error <> '') || ' failed, ' ||
				(SELECT COALESCE(sum(people_new), 0) FROM event_reads er WHERE er.run_id = r.id) || ' new people from reads'
			FROM runs r ORDER BY r.id DESC LIMIT 8`},
		{"Jobs waiting or running", `
			SELECT kind || ' ' || status, count(*)::text FROM jobs WHERE status IN ('queued', 'running')
			GROUP BY kind, status ORDER BY kind, status`},
		{"Job errors in the last 3 days", `
			SELECT kind || ': ' || left(last_error, 200), count(*)::text FROM jobs
			WHERE last_error <> '' AND updated_at > now() - interval '3 days'
			GROUP BY kind, left(last_error, 200) ORDER BY count(*) DESC LIMIT 20`},
		{"Failed checks in the last 3 days", `
			SELECT s.name || ' (' || COALESCE(s.url, '') || ')', left(c.error, 200) FROM source_checks c JOIN sources s ON s.id = c.source_id
			WHERE c.error <> '' AND c.checked_at > now() - interval '3 days' ORDER BY c.checked_at DESC LIMIT 30`},
		{"Failed reads in the last 3 days", `
			SELECT url, left(error, 200) FROM event_reads WHERE error <> '' AND read_at > now() - interval '3 days'
			ORDER BY read_at DESC LIMIT 30`},
		{"Slowest reads in the last 3 days", `
			SELECT url, (duration_ms / 1000) || ' s, ' || people_found || ' people' FROM event_reads
			WHERE error = '' AND read_at > now() - interval '3 days' ORDER BY duration_ms DESC LIMIT 10`},
	}
	for _, sec := range sections {
		rows, err := pool.Query(ctx, sec.sql)
		if err != nil {
			return fmt.Errorf("%s: %w", sec.title, err)
		}
		fmt.Fprintf(w, "## %s\n\n", sec.title)
		n := 0
		for rows.Next() {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				rows.Close()
				return err
			}
			fmt.Fprintf(w, "- %s: %s\n", oneLine(a), oneLine(b))
			n++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintln(w, "- none")
		}
		fmt.Fprintln(w)
	}
	return nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
