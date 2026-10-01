package importer_test

import (
	"testing"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/importer"
)

func TestImportStartingData(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()

	res, err := importer.Import(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesAdded < 100 || res.SourcesSkipped != 1 || res.SearchesAdded != 88 || res.SeedsAdded != 17 {
		t.Fatalf("first import: %s", res)
	}

	counts := map[string]int{}
	rows, _ := pool.Query(ctx, `SELECT status, count(*) FROM sources WHERE kind <> 'search_query' GROUP BY status`)
	for rows.Next() {
		var s string
		var n int
		rows.Scan(&s, &n)
		counts[s] = n
	}
	rows.Close()
	want := map[string]int{"active": 17, "probation": 21, "candidate": 72, "manual": 18, "retired": 7}
	for s, n := range want {
		if counts[s] != n {
			t.Errorf("%d %s sources, want %d", counts[s], s, n)
		}
	}

	checks := []struct {
		sql  string
		want any
	}{
		{`SELECT kind FROM sources WHERE url = 'https://www.meetup.com/startup-breakfast-koln/'`, "calendar_meetup"},
		{`SELECT kind FROM sources WHERE url = 'https://luma.com/theofflineclubcologne'`, "calendar_luma"},
		{`SELECT status FROM sources WHERE name = 'TEDxKoeln'`, "retired"},
		{`SELECT kind FROM sources WHERE name = 'Gateway startups'`, "portfolio"},
		{`SELECT kind || ' ' || city FROM sources WHERE name = 'DGfC coaches near Köln'`, "directory Köln"},
		{`SELECT kind || ' ' || city FROM sources WHERE name = 'Literaturhaus Bonn'`, "calendar_ical Bonn"},
		{`SELECT count(*)::int FROM sources WHERE kind = 'directory' AND status = 'candidate'`, int32(13)},
		{`SELECT status FROM sources WHERE name = 'PechaKucha Night Köln'`, "manual"},
		{`SELECT city FROM sources WHERE name = 'Fuckup Nights Cologne'`, "Köln"},
		{`SELECT count(*)::int FROM sources WHERE name IN ('Frauen gründen anders', 'Kölner Vorbildunternehmerinnen')`, int32(2)},
		{`SELECT count(*)::int FROM seeds WHERE processed_at IS NULL`, int32(2)},
		{`SELECT status || ' ' || city || ' ' || category FROM sources WHERE query = 'BJJ Gym Köln'`, "candidate Köln Search"},
		{`SELECT count(*)::int FROM sources WHERE kind = 'search_query' AND city = ''`, int32(0)},
	}
	for _, c := range checks {
		var got any
		if err := pool.QueryRow(ctx, c.sql).Scan(&got); err != nil {
			t.Errorf("%s: %v", c.sql, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.sql, got, c.want)
		}
	}

	// Tim changes a source, then the import runs again. Nothing changes.
	pool.Exec(ctx, `UPDATE sources SET status = 'retired' WHERE url = 'https://www.startplatz.de/events'`)
	res, err = importer.Import(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesAdded != 0 || res.SearchesAdded != 0 || res.SeedsAdded != 0 || res.SearchesKept != 88 {
		t.Errorf("second import added rows: %s", res)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM sources WHERE url = 'https://www.startplatz.de/events'`).Scan(&status)
	if status != "retired" {
		t.Error("the import overwrote Tim's change")
	}
}
