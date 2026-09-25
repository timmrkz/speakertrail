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
	"github.com/jackc/pgx/v5/pgxpool"

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
	// Checks left over from an earlier run are replaced by this one.
	if _, err := p.Pool.Exec(ctx, `
		UPDATE jobs SET status = 'failed', last_error = 'replaced by run ' || $1, locked_until = NULL, updated_at = $2
		WHERE kind = $3 AND status = 'queued' AND key NOT LIKE 'run:' || $1 || ':%'`,
		fmt.Sprint(runID), now, KindCheckSource); err != nil {
		return 0, err
	}
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

// RunNow starts a run by hand: every due source, as the nightly run would
// check them, worked on by the worker in serve. While a run is still going
// it returns that one instead, so a second click starts nothing new.
func (p *Pipeline) RunNow(ctx context.Context) (runID int64, started bool, err error) {
	// One caller at a time decides, so two clicks cannot start two runs.
	// A caller waits without holding a connection, because the one that
	// decides needs more of them than the one it holds.
	var conn *pgxpool.Conn
	for {
		if conn, err = p.Pool.Acquire(ctx); err != nil {
			return 0, false, err
		}
		var got bool
		if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('speakertrail:run-now'))`).Scan(&got); err != nil {
			conn.Release()
			return 0, false, err
		}
		if got {
			break
		}
		conn.Release()
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			return 0, false, ctx.Err()
		}
	}
	defer conn.Release()
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext('speakertrail:run-now'))`)

	err = conn.QueryRow(ctx, `
		SELECT r.id FROM runs r
		WHERE r.kind IN ('nightly', 'manual') AND r.finished_at IS NULL AND EXISTS (
			SELECT 1 FROM jobs j WHERE j.kind = $1 AND j.status IN ('queued', 'running') AND j.key LIKE 'run:' || r.id || ':%')
		ORDER BY r.id DESC LIMIT 1`, KindCheckSource).Scan(&runID)
	if err == nil {
		return runID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	if runID, err = p.StartRun(ctx, "manual"); err != nil {
		return 0, false, err
	}
	n, err := p.EnqueueDue(ctx, runID)
	if err != nil {
		return 0, false, err
	}
	p.log().Info("run started by hand", "run", runID, "sources", n)
	return runID, true, nil
}

// CheckNow queues one source for an immediate check in its own run.
func (p *Pipeline) CheckNow(ctx context.Context, sourceID int64) (int64, error) {
	runID, err := p.StartRun(ctx, "check")
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
	werr := p.workWithRetries(ctx, w, runID)
	// Record the end even when the context was cancelled.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := p.FinishRun(finishCtx, runID); err != nil {
		return err
	}
	p.log().Info("nightly run finished", "run", runID)
	return werr
}

// retryWait is how long a run waits for retries of its own checks. It
// covers the 1 and 10 minute backoffs, not the hourly ones.
const retryWait = 11 * time.Minute

// workWithRetries works until idle, then waits for this run's checks that
// retry soon, so a short outage of a site does not cost it a night.
func (p *Pipeline) workWithRetries(ctx context.Context, w Worker, runID int64) error {
	for {
		if err := w.RunUntilIdle(ctx); err != nil {
			return err
		}
		var next *time.Time
		err := p.Pool.QueryRow(ctx, `
			SELECT min(run_after) FROM jobs
			WHERE status = 'queued' AND key LIKE 'run:' || $1 || ':%' AND run_after <= $2`,
			fmt.Sprint(runID), p.now().Add(retryWait)).Scan(&next)
		if err != nil {
			return err
		}
		if next == nil {
			return nil
		}
		wait := time.Until(*next) + time.Second
		p.log().Info("waiting for retries", "run", runID, "wait", wait.Round(time.Second))
		select {
		case <-time.After(max(wait, 0)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
