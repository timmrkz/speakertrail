package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventPages picks up to n pages of single upcoming events that runs have
// found, one per source so the sample covers different organisers. It
// leaves out pages that are a source themselves, because a listing names
// its speakers far less often than the event's own page.
func EventPages(ctx context.Context, pool *pgxpool.Pool, n int, now time.Time) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT url FROM (
			SELECT DISTINCT ON (si.source_id) e.canonical_url AS url, e.starts_at
			FROM events e JOIN sightings si ON si.event_id = e.id
			WHERE e.canonical_url <> '' AND e.fit = 'kept' AND e.format <> 'online' AND e.starts_at >= $1
			  AND e.canonical_url NOT LIKE '%.ics'
			  AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.url = e.canonical_url)
			ORDER BY si.source_id, e.starts_at, e.id) per_source
		ORDER BY starts_at, url
		LIMIT $2`, now, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
