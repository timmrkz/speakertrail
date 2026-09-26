package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/dbtest"
)

// The report covers runs, failures and waiting work, and never names a
// person, so Tim can attach it to a chat.
func TestReportShowsFailuresButNoNames(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	now := time.Now()
	for _, sql := range []string{
		`INSERT INTO sources (name, kind, url, status) VALUES ('Beispiel Events', 'listing', 'https://example.org/events', 'active')`,
		`INSERT INTO runs (kind, started_at, finished_at) VALUES ('manual', now() - interval '5 minutes', now())`,
		`INSERT INTO source_checks (run_id, source_id, error, checked_at) VALUES (1, 1, 'get https://example.org/events: status 503', now())`,
		`INSERT INTO events (title, starts_at, canonical_url, fit) VALUES ('Pitch Abend', now() + interval '3 days', 'https://example.org/e/1', 'kept')`,
		`INSERT INTO event_reads (run_id, event_id, url, error, read_at) VALUES (1, 1, 'https://example.org/e/1', 'the language model failed, it answered 500: Compute error.', now())`,
		`INSERT INTO people (full_name, normalised_name, fit) VALUES ('Lena Musterfrau', 'lena musterfrau', 'founder')`,
		`INSERT INTO jobs (kind, key, status, last_error, run_after, created_at, updated_at) VALUES ('read_event', 'run:1:read:1', 'failed', 'stopped by hand', now(), now(), now())`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var b bytes.Buffer
	if err := writeReport(ctx, pool, &b, now); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"1 manual", "1 checks, 1 failed, 1 reads, 1 failed",
		"Beispiel Events (https://example.org/events): get https://example.org/events: status 503",
		"Compute error.", "read_event: stopped by hand", "events waiting for a read: 1", "people with a founder role: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Musterfrau") {
		t.Error("the report names a person")
	}
}
