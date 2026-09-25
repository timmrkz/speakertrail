package db_test

import (
	"testing"

	"github.com/timmrkz/speakertrail/internal/db"
	"github.com/timmrkz/speakertrail/internal/dbtest"
)

func TestMigrationsUpDownUp(t *testing.T) {
	pool := dbtest.NewEmpty(t)
	ctx := t.Context()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if err := db.Reset(ctx, pool); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("second up: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM settings").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("settings have no default rows")
	}
}

func TestSchemaRules(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()

	bad := map[string]string{
		"search source without query": `INSERT INTO sources (kind, url) VALUES ('search_query', 'https://example.com')`,
		"page source without url":     `INSERT INTO sources (kind) VALUES ('listing')`,
		"unknown source status":       `INSERT INTO sources (kind, url, status) VALUES ('listing', 'https://example.com', 'maybe')`,
		"profile with two owners": `WITH p AS (INSERT INTO people (full_name, normalised_name) VALUES ('A', 'a') RETURNING id),
			o AS (INSERT INTO organisations (name, normalised_name) VALUES ('B', 'b') RETURNING id)
			INSERT INTO profiles (person_id, organisation_id, platform, url) SELECT p.id, o.id, 'website', 'https://b.example' FROM p, o`,
		"sighting of nothing": `WITH s AS (INSERT INTO sources (kind, url) VALUES ('listing', 'https://c.example') RETURNING id)
			INSERT INTO sightings (source_id, checked_at, is_new) SELECT id, now(), true FROM s`,
	}
	for name, sql := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, sql); err == nil {
				t.Error("insert succeeded, want a constraint error")
			}
		})
	}

	if _, err := pool.Exec(ctx, `INSERT INTO sources (kind, query) VALUES ('search_query', 'Pitch Night Köln')`); err != nil {
		t.Fatalf("search source: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (kind, query) VALUES ('search_query', 'pitch night köln')`); err == nil {
		t.Error("the same search query was stored twice")
	}
}
