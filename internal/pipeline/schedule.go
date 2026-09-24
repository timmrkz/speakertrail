package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/queue"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// StartRun records the start of a run.
func (p *Pipeline) StartRun(ctx context.Context, kind string) (int64, error) {
	var id int64
	err := p.Pool.QueryRow(ctx, `INSERT INTO runs (kind, started_at) VALUES ($1, $2) RETURNING id`, kind, p.now()).Scan(&id)
	return id, err
}

// FinishRun records the end of a run.
func (p *Pipeline) FinishRun(ctx context.Context, runID int64) error {
	_, err := p.Pool.Exec(ctx, `UPDATE runs SET finished_at = $2 WHERE id = $1`, runID, p.now())
	return err
}

// EnqueueDue queues a check for every source that is due, the first checks
// of a limited number of new candidates, the open seeds and the pruning.
// It returns the number of sources queued.
func (p *Pipeline) EnqueueDue(ctx context.Context, runID int64) (int, error) {
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return 0, err
	}
	now := p.now()
	rows, err := p.Pool.Query(ctx, `
		(SELECT id FROM sources
		 WHERE url IS NOT NULL AND status IN ('active', 'probation', 'retired')
		   AND (next_check_at IS NULL OR next_check_at <= $1)
		 ORDER BY points DESC, id)
		UNION ALL
		(SELECT id FROM sources
		 WHERE url IS NOT NULL AND status = 'candidate' AND (next_check_at IS NULL OR next_check_at <= $1)
		 ORDER BY checks, created_at, id
		 LIMIT $2)`, now, cfg.Int("new_candidates_per_run", 10))
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := p.Queue.Enqueue(ctx, queue.NewJob{
			Kind: KindCheckSource, Key: fmt.Sprintf("run:%d:source:%d", runID, id),
			Payload: CheckPayload{SourceID: id, RunID: runID},
		}); err != nil {
			return 0, err
		}
	}

	seeds, err := p.Pool.Query(ctx, `SELECT id FROM seeds WHERE processed_at IS NULL ORDER BY id`)
	if err != nil {
		return 0, err
	}
	seedIDs, err := pgx.CollectRows(seeds, pgx.RowTo[int64])
	if err != nil {
		return 0, err
	}
	for _, id := range seedIDs {
		if err := p.EnqueueSeed(ctx, id); err != nil {
			return 0, err
		}
	}
	if _, err := p.Queue.Enqueue(ctx, queue.NewJob{Kind: KindPrune, Key: "prune:" + now.Format("2006-01-02")}); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// CheckNow queues one source for an immediate check in its own run.
func (p *Pipeline) CheckNow(ctx context.Context, sourceID int64) (int64, error) {
	runID, err := p.StartRun(ctx, "manual")
	if err != nil {
		return 0, err
	}
	_, err = p.Queue.Enqueue(ctx, queue.NewJob{
		Kind: KindCheckSource, Key: fmt.Sprintf("run:%d:source:%d", runID, sourceID),
		Payload: CheckPayload{SourceID: sourceID, RunID: runID},
	})
	return runID, err
}

// Worker runs queued jobs.
type Worker interface {
	RunUntilIdle(ctx context.Context) error
}

// Nightly runs one full pass: queue what is due, work until nothing is
// left, and record the run.
func (p *Pipeline) Nightly(ctx context.Context, w Worker) error {
	runID, err := p.StartRun(ctx, "nightly")
	if err != nil {
		return err
	}
	n, err := p.EnqueueDue(ctx, runID)
	if err != nil {
		return err
	}
	p.log().Info("nightly run started", "run", runID, "sources", n)
	werr := w.RunUntilIdle(ctx)
	// Record the end even when the context was cancelled.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := p.FinishRun(finishCtx, runID); err != nil {
		return err
	}
	p.log().Info("nightly run finished", "run", runID)
	return werr
}

// EnqueueSeed queues the processing of one seed.
func (p *Pipeline) EnqueueSeed(ctx context.Context, seedID int64) error {
	_, err := p.Queue.Enqueue(ctx, queue.NewJob{Kind: KindSeed, Key: fmt.Sprintf("seed:%d", seedID), Payload: map[string]int64{"seed_id": seedID}})
	return err
}

// handleSeed turns a pasted link into a candidate source. Names wait for
// the search API, and LinkedIn or Instagram links are explained, not
// followed.
func (p *Pipeline) handleSeed(ctx context.Context, j *queue.Job) error {
	var pl struct {
		SeedID int64 `json:"seed_id"`
	}
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return err
	}
	var input string
	var processed *time.Time
	err := p.Pool.QueryRow(ctx, `SELECT input, processed_at FROM seeds WHERE id = $1`, pl.SeedID).Scan(&input, &processed)
	if errors.Is(err, pgx.ErrNoRows) || processed != nil {
		return nil
	}
	if err != nil {
		return err
	}
	result, done, err := p.seedToSource(ctx, pl.SeedID, input)
	if err != nil {
		return err
	}
	var at *time.Time
	if done {
		now := p.now()
		at = &now
	}
	_, err = p.Pool.Exec(ctx, `UPDATE seeds SET result = $2, processed_at = $3 WHERE id = $1`, pl.SeedID, result, at)
	return err
}

func (p *Pipeline) seedToSource(ctx context.Context, seedID int64, input string) (result string, done bool, err error) {
	link := firstURL(input)
	if link == "" {
		return "Waiting for the name search, which comes with the search API", false, nil
	}
	if errors.Is(fetch.CheckAllowed(link), fetch.ErrBlockedHost) {
		return "LinkedIn and Instagram links cannot be followed. Paste the organiser's website or event page instead", true, nil
	}
	if strings.Contains(link, "facebook.com") {
		return "Facebook pages cannot be checked. Paste the organiser's website or event page instead", true, nil
	}
	cal := CalendarURL(link)
	if cal == "" {
		cal = link
	}
	var id int64
	var status string
	err = p.Pool.QueryRow(ctx, `SELECT id, status FROM sources WHERE url = $1`, cal).Scan(&id, &status)
	if err == nil {
		return fmt.Sprintf("Already a source (%s): %s", status, cal), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	err = p.Pool.QueryRow(ctx, `
		INSERT INTO sources (name, kind, url, status, discovered_from_seed_id, discovered_note)
		VALUES ($1, $2, $3, 'candidate', $4, 'Pasted by Tim') RETURNING id`,
		NameFromURL(cal), SourceKindFor(cal), cal, seedID).Scan(&id)
	if err != nil {
		return "", false, err
	}
	return "Added as a candidate source: " + cal, true, nil
}

func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		f = strings.Trim(f, "<>()[]\"'")
		if !strings.HasPrefix(f, "http://") && !strings.HasPrefix(f, "https://") {
			if strings.HasPrefix(f, "www.") {
				f = "https://" + f
			} else {
				continue
			}
		}
		if u, err := url.Parse(f); err == nil && u.Host != "" {
			return u.String()
		}
	}
	return ""
}

// handlePrune deletes stored pages after their retention, old finished
// jobs, and skipped people after their retention.
func (p *Pipeline) handlePrune(ctx context.Context, _ *queue.Job) error {
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	now := p.now()
	steps := []struct {
		sql string
		arg time.Time
	}{
		{`DELETE FROM fetches WHERE fetched_at < $1`, now.Add(-cfg.Days("snapshot_retention_days", 30))},
		{`DELETE FROM jobs WHERE status = 'done' AND updated_at < $1`, now.Add(-30 * 24 * time.Hour)},
		{`DELETE FROM people WHERE podcast_status = 'skipped' AND status_changed_at < $1`, now.Add(-cfg.Days("skipped_person_retention_days", 90))},
	}
	for _, s := range steps {
		if _, err := p.Pool.Exec(ctx, s.sql, s.arg); err != nil {
			return err
		}
	}
	return nil
}
