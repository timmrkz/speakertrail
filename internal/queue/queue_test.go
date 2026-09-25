package queue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/dbtest"
	"github.com/timmrkz/speakertrail/internal/queue"
)

// clock is a time source the test moves by hand.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock {
	return &clock{t: time.Date(2026, 9, 25, 5, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func count(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM jobs WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepeatedKeyIsNeverQueuedTwice(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	q := queue.New(pool, queue.Options{})
	job := queue.NewJob{Kind: "fetch_page", Key: "https://www.startplatz.de/events"}

	added, err := q.Enqueue(ctx, job)
	if err != nil || !added {
		t.Fatalf("first enqueue: added %v, error %v", added, err)
	}
	added, err = q.Enqueue(ctx, job)
	if err != nil || added {
		t.Fatalf("second enqueue: added %v, error %v, want not added", added, err)
	}

	// Many workers enqueueing the same key at once still make one job.
	var wg sync.WaitGroup
	var addedCount atomic.Int32
	for range 20 {
		wg.Go(func() {
			ok, err := q.Enqueue(ctx, queue.NewJob{Kind: "fetch_page", Key: "https://luma.com/theofflineclubcologne"})
			if err != nil {
				t.Error(err)
			}
			if ok {
				addedCount.Add(1)
			}
		})
	}
	wg.Wait()
	if n := addedCount.Load(); n != 1 {
		t.Errorf("concurrent enqueue added %d jobs, want 1", n)
	}
	if n := count(t, pool, "kind = 'fetch_page'"); n != 2 {
		t.Errorf("%d fetch_page jobs in the table, want 2", n)
	}

	// The same key with another kind is different work.
	if added, _ := q.Enqueue(ctx, queue.NewJob{Kind: "extract_page", Key: job.Key}); !added {
		t.Error("same key with another kind was not added")
	}

	// A running job is not queued again either.
	j, err := q.Claim(ctx)
	if err != nil || j == nil {
		t.Fatalf("claim: %v, %v", j, err)
	}
	if added, _ := q.Enqueue(ctx, queue.NewJob{Kind: j.Kind, Key: j.Key}); added {
		t.Error("a running job was queued again")
	}

	// Once done, the same key can be queued for the next run.
	if err := q.Complete(ctx, j); err != nil {
		t.Fatal(err)
	}
	if added, _ := q.Enqueue(ctx, queue.NewJob{Kind: j.Kind, Key: j.Key}); !added {
		t.Error("a finished job could not be queued for the next run")
	}
	if n := count(t, pool, "kind = $1 AND key = $2", j.Kind, j.Key); n != 1 {
		t.Errorf("%d rows for one key, want 1", n)
	}
}

func TestFailingJobRetriesThenStops(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	clk := newClock()
	q := queue.New(pool, queue.Options{Now: clk.Now})

	if _, err := q.Enqueue(ctx, queue.NewJob{Kind: "check_source", Key: "42"}); err != nil {
		t.Fatal(err)
	}

	// Waits after attempts 1 to 4. Attempt 5 is the last.
	waits := []time.Duration{time.Minute, 10 * time.Minute, time.Hour, time.Hour}
	for attempt := 1; attempt <= 5; attempt++ {
		j, err := q.Claim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if j == nil {
			t.Fatalf("attempt %d: no job due", attempt)
		}
		if j.Attempts != attempt {
			t.Fatalf("attempt %d: job says attempt %d", attempt, j.Attempts)
		}
		if err := q.Fail(ctx, j, fmt.Errorf("boom %d", attempt)); err != nil {
			t.Fatal(err)
		}
		if attempt == 5 {
			break
		}

		wait := waits[attempt-1]
		clk.Add(wait - time.Second)
		if j, _ := q.Claim(ctx); j != nil {
			t.Fatalf("attempt %d: job came back before its %v backoff", attempt, wait)
		}
		clk.Add(time.Second)
	}

	clk.Add(24 * time.Hour)
	if j, _ := q.Claim(ctx); j != nil {
		t.Fatal("job ran again after 5 attempts")
	}
	var status, lastError string
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT status, attempts, last_error FROM jobs WHERE key = '42'").
		Scan(&status, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != queue.StatusFailed || attempts != 5 || lastError != "boom 5" {
		t.Errorf("got status %q, attempts %d, error %q, want failed, 5, boom 5", status, attempts, lastError)
	}
}

func TestExpiredLeaseIsTakenOver(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	clk := newClock()
	q := queue.New(pool, queue.Options{Now: clk.Now, Lease: time.Minute})

	if _, err := q.Enqueue(ctx, queue.NewJob{Kind: "fetch_page", Key: "a"}); err != nil {
		t.Fatal(err)
	}
	first, _ := q.Claim(ctx)
	if first == nil {
		t.Fatal("no job")
	}
	if j, _ := q.Claim(ctx); j != nil {
		t.Fatal("a job with a running lease was claimed twice")
	}

	// The first worker hangs. After the lease, another worker takes over.
	clk.Add(2 * time.Minute)
	second, _ := q.Claim(ctx)
	if second == nil || second.ID != first.ID {
		t.Fatalf("expired job was not taken over: %+v", second)
	}
	if err := q.Complete(ctx, first); !errors.Is(err, queue.ErrLostLease) {
		t.Errorf("stale worker completed the job, error %v", err)
	}
	if err := q.Complete(ctx, second); err != nil {
		t.Errorf("new owner could not complete: %v", err)
	}
}

func TestTwoWorkersNeverTakeTheSameJob(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	q := queue.New(pool, queue.Options{})

	const jobs = 300
	for i := range jobs {
		if _, err := q.Enqueue(ctx, queue.NewJob{Kind: "fetch_page", Key: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	seen := map[int64]int{}
	handler := func(ctx context.Context, j *queue.Job) error {
		mu.Lock()
		seen[j.ID]++
		mu.Unlock()
		time.Sleep(time.Millisecond)
		return nil
	}

	// Two worker processes with several goroutines each, all claiming from
	// the same table.
	var wg sync.WaitGroup
	for range 2 {
		w := &queue.Worker{
			Queue:        queue.New(pool, queue.Options{}),
			Handlers:     map[string]queue.Handler{"fetch_page": handler},
			Concurrency:  4,
			PollInterval: 10 * time.Millisecond,
		}
		wg.Go(func() {
			if err := w.RunUntilIdle(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if len(seen) != jobs {
		t.Errorf("%d jobs ran, want %d", len(seen), jobs)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("job %d ran %d times", id, n)
		}
	}
	if n := count(t, pool, "status = 'done'"); n != jobs {
		t.Errorf("%d jobs done, want %d", n, jobs)
	}
}

func TestWorkerRecordsFailuresAndPanics(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	q := queue.New(pool, queue.Options{})
	for _, kind := range []string{"fails", "panics", "unknown"} {
		if _, err := q.Enqueue(ctx, queue.NewJob{Kind: kind, Key: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	w := &queue.Worker{
		Queue: q,
		Handlers: map[string]queue.Handler{
			"fails":  func(context.Context, *queue.Job) error { return errors.New("site blocked") },
			"panics": func(context.Context, *queue.Job) error { panic("parser bug") },
		},
	}
	if err := w.RunUntilIdle(ctx); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"fails": "site blocked", "panics": "panic: parser bug", "unknown": `no handler for job kind "unknown"`}
	for kind, prefix := range want {
		var status, lastError string
		if err := pool.QueryRow(ctx, "SELECT status, last_error FROM jobs WHERE kind = $1", kind).Scan(&status, &lastError); err != nil {
			t.Fatal(err)
		}
		if status != queue.StatusQueued || len(lastError) < len(prefix) || lastError[:len(prefix)] != prefix {
			t.Errorf("%s: status %q, error %q, want queued for retry with %q", kind, status, lastError, prefix)
		}
	}
}

func TestSiteLimiterSpacesRequestsPerWebsite(t *testing.T) {
	pool := dbtest.New(t)
	ctx := t.Context()
	clk := newClock()
	l := queue.NewSiteLimiter(pool, 5*time.Second, clk.Now)

	var slots []time.Time
	for _, u := range []string{
		"https://www.startplatz.de/events",
		"https://startplatz.de/events?page=2",
		"https://STARTPLATZ.de:443/koeln",
	} {
		s, err := l.Reserve(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		slots = append(slots, s)
	}
	for i, s := range slots {
		if want := clk.Now().Add(time.Duration(i) * 5 * time.Second); !s.Equal(want) {
			t.Errorf("request %d on the same site at %v, want %v", i+1, s, want)
		}
	}

	other, err := l.Reserve(ctx, "https://luma.com/theofflineclubcologne")
	if err != nil {
		t.Fatal(err)
	}
	if !other.Equal(clk.Now()) {
		t.Errorf("another site had to wait until %v", other)
	}

	// After a quiet minute the site is free at once again.
	clk.Add(time.Minute)
	s, err := l.Reserve(ctx, "https://www.startplatz.de/events")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Equal(clk.Now()) {
		t.Errorf("idle site had to wait until %v", s)
	}
}

func TestSiteOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.Startplatz.de/events": "startplatz.de",
		"http://luma.com:8080/x":           "luma.com",
		"https://cologne.aitinkerers.org/": "cologne.aitinkerers.org",
	} {
		got, err := queue.SiteOf(in)
		if err != nil || got != want {
			t.Errorf("SiteOf(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	if _, err := queue.SiteOf("not a url"); err == nil {
		t.Error("SiteOf accepted a URL without host")
	}
}
