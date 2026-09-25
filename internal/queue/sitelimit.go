package queue

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SiteLimiter keeps every website at one request per interval, across all
// workers. Each request reserves the next free slot in the database and then
// waits for it.
type SiteLimiter struct {
	pool     *pgxpool.Pool
	interval time.Duration
	now      func() time.Time
}

// NewSiteLimiter returns a limiter with the given interval between two
// requests to the same website. now may be nil.
func NewSiteLimiter(pool *pgxpool.Pool, interval time.Duration, now func() time.Time) *SiteLimiter {
	if now == nil {
		now = time.Now
	}
	return &SiteLimiter{pool: pool, interval: interval, now: now}
}

// Reserve books the next free request slot for the website of rawURL and
// returns when it starts.
func (l *SiteLimiter) Reserve(ctx context.Context, rawURL string) (time.Time, error) {
	host, err := SiteOf(rawURL)
	if err != nil {
		return time.Time{}, err
	}
	var slot time.Time
	err = l.pool.QueryRow(ctx, `
		INSERT INTO site_slots (host, next_request_at)
		VALUES ($1, $2::timestamptz + make_interval(secs => $3))
		ON CONFLICT (host) DO UPDATE SET
			next_request_at = greatest(site_slots.next_request_at, $2::timestamptz) + make_interval(secs => $3)
		RETURNING next_request_at - make_interval(secs => $3)`,
		host, l.now(), l.interval.Seconds()).Scan(&slot)
	if err != nil {
		return time.Time{}, fmt.Errorf("reserve slot for %s: %w", host, err)
	}
	return slot, nil
}

// Wait reserves a slot for the website of rawURL and sleeps until it starts.
func (l *SiteLimiter) Wait(ctx context.Context, rawURL string) error {
	slot, err := l.Reserve(ctx, rawURL)
	if err != nil {
		return err
	}
	d := slot.Sub(l.now())
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SiteOf returns the website a URL belongs to: its host in lower case,
// without port and without a leading "www.".
func SiteOf(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("site of %q: %w", rawURL, err)
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "" {
		return "", fmt.Errorf("site of %q: no host", rawURL)
	}
	return host, nil
}
