// Package importer loads the starting data from the brief: the source list,
// the manual and retired sources, and the seeds. Importing twice changes
// nothing, and it never touches a row that already exists, so the engine's
// lifecycle and Tim's edits survive.
package importer

import (
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/pipeline"
)

//go:embed data/sources.csv
var sourcesCSV string

//go:embed data/seeds.csv
var seedsCSV string

// Result counts what an import did.
type Result struct {
	SourcesAdded, SourcesKept, SourcesSkipped int
	SeedsAdded, SeedsKept                     int
}

func (r Result) String() string {
	return fmt.Sprintf("sources: %d added, %d already there, %d without an address skipped. seeds: %d added, %d already there",
		r.SourcesAdded, r.SourcesKept, r.SourcesSkipped, r.SeedsAdded, r.SeedsKept)
}

// Import loads the starting data.
func Import(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	var res Result
	rows, err := readCSV(sourcesCSV)
	if err != nil {
		return res, fmt.Errorf("sources.csv: %w", err)
	}
	for _, r := range rows {
		name, url, status := r["name"], strings.TrimSpace(r["url"]), r["status"]
		if url == "" && status != "manual" && status != "retired" {
			res.SourcesSkipped++
			continue
		}
		var exists bool
		if url != "" {
			err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sources WHERE url = $1)`, url).Scan(&exists)
		} else {
			err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sources WHERE url IS NULL AND lower(name) = lower($1))`, name).Scan(&exists)
		}
		if err != nil {
			return res, err
		}
		if exists {
			res.SourcesKept++
			continue
		}
		kind := "listing"
		var u *string
		if url != "" {
			kind = pipeline.SourceKindFor(url)
			u = &url
		}
		notes := r["notes"]
		if fit := r["fit"]; fit != "" {
			notes = "Fit " + strings.ToLower(fit) + ". " + notes
		}
		city := extract.CityOf(name, url)
		// A retired source waits for its recheck instead of running tonight.
		var next *time.Time
		if status == "retired" {
			t := time.Now().Add(28 * 24 * time.Hour)
			next = &t
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO sources (name, kind, url, category, city, notes, status, next_check_at, discovered_note)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'Imported from the brief')`,
			name, kind, u, r["category"], city, notes, status, next); err != nil {
			return res, fmt.Errorf("source %q: %w", name, err)
		}
		res.SourcesAdded++
	}

	seeds, err := readCSV(seedsCSV)
	if err != nil {
		return res, fmt.Errorf("seeds.csv: %w", err)
	}
	for _, s := range seeds {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM seeds WHERE input = $1)`, s["input"]).Scan(&exists); err != nil {
			return res, err
		}
		if exists {
			res.SeedsKept++
			continue
		}
		// Ticked seeds were followed by hand in the first run.
		var processed *time.Time
		result := ""
		if s["done"] == "yes" {
			t := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
			processed, result = &t, "Checked by hand in the first run"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO seeds (input, processed_at, result) VALUES ($1, $2, $3)`, s["input"], processed, result); err != nil {
			return res, err
		}
		res.SeedsAdded++
	}
	return res, nil
}

func readCSV(data string) ([]map[string]string, error) {
	records, err := csv.NewReader(strings.NewReader(data)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	head := records[0]
	var out []map[string]string
	for _, rec := range records[1:] {
		m := map[string]string{}
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}
