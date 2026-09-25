// Package queue is the job queue in PostgreSQL.
//
// Kind plus key is unique, so the same work is never queued twice. A worker
// claims a job with FOR UPDATE SKIP LOCKED and holds it by a lease, not by an
// open transaction, so no transaction stays open while a job runs. A failed
// job is retried after 1 minute, 10 minutes and 1 hour, and stops after 5
// attempts.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values of a job.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// DefaultBackoff is the wait after the first, second, third and every later
// failed attempt.
var DefaultBackoff = []time.Duration{time.Minute, 10 * time.Minute, time.Hour}

// Options tune a queue. Zero values take the defaults.
type Options struct {
	// MaxAttempts is how often a job runs before it stays failed. Default 5.
	MaxAttempts int
	// Backoff is the wait after each failed attempt. The last entry repeats.
	Backoff []time.Duration
	// Lease is how long a claimed job belongs to its worker. After that
	// another worker may take it over. Default 10 minutes.
	Lease time.Duration
	// Now returns the current time. Tests replace it.
	Now func() time.Time
}

// Queue enqueues, claims and finishes jobs.
type Queue struct {
	pool *pgxpool.Pool
	opts Options
}

// New returns a queue on the given pool.
func New(pool *pgxpool.Pool, opts Options) *Queue {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 5
	}
	if len(opts.Backoff) == 0 {
		opts.Backoff = DefaultBackoff
	}
	if opts.Lease <= 0 {
		opts.Lease = 10 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Queue{pool: pool, opts: opts}
}

// Job is a claimed job.
type Job struct {
	ID       int64
	Kind     string
	Key      string
	Payload  json.RawMessage
	Attempts int
}

// NewJob describes a job to enqueue.
type NewJob struct {
	Kind    string
	Key     string
	Payload any
	// RunAfter delays the job. The zero value means now.
	RunAfter time.Time
}

// Enqueue adds a job and reports whether it was added. A job with the same
// kind and key that is queued or running is left alone, so enqueueing is
// always safe to repeat. A finished or failed job with the same kind and key
// is queued again from scratch.
func (q *Queue) Enqueue(ctx context.Context, j NewJob) (bool, error) {
	if j.Kind == "" || j.Key == "" {
		return false, errors.New("enqueue: kind and key are required")
	}
	payload := []byte("{}")
	if j.Payload != nil {
		var err error
		if payload, err = json.Marshal(j.Payload); err != nil {
			return false, fmt.Errorf("enqueue %s %s: payload: %w", j.Kind, j.Key, err)
		}
	}
	now := q.opts.Now()
	runAfter := j.RunAfter
	if runAfter.IsZero() {
		runAfter = now
	}
	var id int64
	err := q.pool.QueryRow(ctx, `
		INSERT INTO jobs (kind, key, payload, run_after, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (kind, key) DO UPDATE SET
			payload = EXCLUDED.payload,
			status = 'queued',
			attempts = 0,
			run_after = EXCLUDED.run_after,
			locked_until = NULL,
			last_error = '',
			updated_at = EXCLUDED.updated_at
		WHERE jobs.status IN ('done', 'failed')
		RETURNING id`,
		j.Kind, j.Key, payload, runAfter, now).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("enqueue %s %s: %w", j.Kind, j.Key, err)
	}
	return true, nil
}

// Claim takes the next job that is due, or a job whose worker's lease ran
// out. It returns nil when nothing is due. Several workers can claim at the
// same time and never get the same job.
func (q *Queue) Claim(ctx context.Context) (*Job, error) {
	now := q.opts.Now()
	// A job whose lease ran out on its last attempt will not run again.
	if _, err := q.pool.Exec(ctx, `
		UPDATE jobs SET status = 'failed', locked_until = NULL, updated_at = $1,
			last_error = CASE WHEN last_error = '' THEN 'lease expired' ELSE last_error END
		WHERE status = 'running' AND locked_until < $1 AND attempts >= $2`,
		now, q.opts.MaxAttempts); err != nil {
		return nil, fmt.Errorf("claim: expire leases: %w", err)
	}
	var j Job
	err := q.pool.QueryRow(ctx, `
		UPDATE jobs SET status = 'running', attempts = attempts + 1, locked_until = $2, updated_at = $1
		WHERE id = (
			SELECT id FROM jobs
			WHERE (status = 'queued' AND run_after <= $1)
			   OR (status = 'running' AND locked_until < $1)
			ORDER BY run_after, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, key, payload, attempts`,
		now, now.Add(q.opts.Lease)).Scan(&j.ID, &j.Kind, &j.Key, &j.Payload, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim: %w", err)
	}
	return &j, nil
}

// ErrLostLease means the job was taken over by another worker because the
// lease ran out, so this worker's result was not recorded.
var ErrLostLease = errors.New("job lease lost")

// Complete marks a claimed job as done.
func (q *Queue) Complete(ctx context.Context, j *Job) error {
	return q.finish(ctx, j, `
		UPDATE jobs SET status = 'done', locked_until = NULL, last_error = '', updated_at = $3
		WHERE id = $1 AND status = 'running' AND attempts = $2`,
		j.ID, j.Attempts, q.opts.Now())
}

// Fail records a failed attempt. The job is queued again after the backoff,
// or stays failed once it has used all attempts.
func (q *Queue) Fail(ctx context.Context, j *Job, cause error) error {
	now := q.opts.Now()
	msg := "unknown error"
	if cause != nil {
		msg = cause.Error()
	}
	if j.Attempts >= q.opts.MaxAttempts {
		return q.finish(ctx, j, `
			UPDATE jobs SET status = 'failed', locked_until = NULL, last_error = $3, updated_at = $4
			WHERE id = $1 AND status = 'running' AND attempts = $2`,
			j.ID, j.Attempts, msg, now)
	}
	return q.finish(ctx, j, `
		UPDATE jobs SET status = 'queued', locked_until = NULL, last_error = $3, run_after = $5, updated_at = $4
		WHERE id = $1 AND status = 'running' AND attempts = $2`,
		j.ID, j.Attempts, msg, now, now.Add(q.backoff(j.Attempts)))
}

func (q *Queue) finish(ctx context.Context, j *Job, sql string, args ...any) error {
	tag, err := q.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("finish job %d: %w", j.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("finish job %d: %w", j.ID, ErrLostLease)
	}
	return nil
}

// backoff is the wait after the given failed attempt, counted from 1.
func (q *Queue) backoff(attempt int) time.Duration {
	i := min(attempt-1, len(q.opts.Backoff)-1)
	return q.opts.Backoff[max(i, 0)]
}
