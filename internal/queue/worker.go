package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Handler runs one job. A returned error counts as a failed attempt.
type Handler func(ctx context.Context, j *Job) error

// Worker claims jobs and runs them with the handler for their kind.
type Worker struct {
	Queue    *Queue
	Handlers map[string]Handler
	// Concurrency is the number of jobs running at the same time. Default 1.
	Concurrency int
	// PollInterval is the wait when no job is due. Default 5 seconds.
	PollInterval time.Duration
	Log          *slog.Logger
}

// Run works on jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	return w.run(ctx, false)
}

// RunUntilIdle works on jobs until none is due and none is running, then
// returns. The nightly scheduled job uses it.
func (w *Worker) RunUntilIdle(ctx context.Context) error {
	return w.run(ctx, true)
}

func (w *Worker) run(ctx context.Context, untilIdle bool) error {
	n := max(w.Concurrency, 1)
	poll := w.PollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	log := w.Log
	if log == nil {
		log = slog.Default()
	}

	var (
		mu      sync.Mutex
		busy    int
		idle    = make(chan struct{})
		once    sync.Once
		wg      sync.WaitGroup
		errOnce sync.Once
		runErr  error
	)
	loopCtx, stop := context.WithCancel(ctx)
	defer stop()

	for range n {
		wg.Go(func() {
			for loopCtx.Err() == nil {
				mu.Lock()
				busy++
				mu.Unlock()
				j, err := w.Queue.Claim(loopCtx)
				if err == nil && j != nil {
					w.runJob(loopCtx, log, j)
				}
				mu.Lock()
				busy--
				nowIdle := busy == 0
				mu.Unlock()

				if err != nil {
					if loopCtx.Err() != nil {
						return
					}
					errOnce.Do(func() { runErr = err })
					log.Error("claim failed", "error", err)
				}
				if j != nil {
					continue
				}
				if untilIdle && nowIdle && err == nil {
					once.Do(func() { close(idle) })
					return
				}
				select {
				case <-loopCtx.Done():
					return
				case <-idle:
					return
				case <-time.After(poll):
				}
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if untilIdle {
		return runErr
	}
	return nil
}

func (w *Worker) runJob(ctx context.Context, log *slog.Logger, j *Job) {
	log = log.With("job", j.ID, "kind", j.Kind, "key", j.Key, "attempt", j.Attempts)
	start := time.Now()
	err := w.call(ctx, j)
	// Record the result even when ctx was cancelled mid-job.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err != nil {
		log.Warn("job failed", "error", err, "duration", time.Since(start))
		if ferr := w.Queue.Fail(finishCtx, j, err); ferr != nil && !errors.Is(ferr, ErrLostLease) {
			log.Error("recording failure failed", "error", ferr)
		}
		return
	}
	log.Info("job done", "duration", time.Since(start))
	if cerr := w.Queue.Complete(finishCtx, j); cerr != nil && !errors.Is(cerr, ErrLostLease) {
		log.Error("recording success failed", "error", cerr)
	}
}

func (w *Worker) call(ctx context.Context, j *Job) (err error) {
	h, ok := w.Handlers[j.Kind]
	if !ok {
		return fmt.Errorf("no handler for job kind %q", j.Kind)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return h(ctx, j)
}
