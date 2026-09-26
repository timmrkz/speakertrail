// Package pipeline runs the crawl: it checks sources, extracts events,
// stores what it found and moves sources through their lifecycle.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/queue"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// Job kinds.
const (
	KindCheckSource = "check_source"
	KindSeed        = "process_seed"
	KindPrune       = "prune"
)

// CheckPayload is the payload of a check_source job.
type CheckPayload struct {
	SourceID int64 `json:"source_id"`
	RunID    int64 `json:"run_id"`
}

// Fetcher is what the pipeline needs from the fetch package.
type Fetcher interface {
	HTTP(ctx context.Context, url string) (*fetch.Page, error)
	Browser(ctx context.Context, url string) (*fetch.Page, error)
	HasBrowser() bool
}

// Pipeline holds what the job handlers share.
type Pipeline struct {
	Pool    *pgxpool.Pool
	Fetcher Fetcher
	Queue   *queue.Queue
	// Reader is the local language model that reads event pages for
	// people. Without it events are not read.
	Reader Reader
	Log    *slog.Logger
	// Now is the current time. Tests set it.
	Now func() time.Time
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pipeline) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

// Handlers returns the job handlers of the pipeline.
func (p *Pipeline) Handlers() map[string]queue.Handler {
	return map[string]queue.Handler{
		KindCheckSource: p.handleCheck,
		KindSeed:        p.handleSeed,
		KindPrune:       p.handlePrune,
		KindReadEvent:   p.handleRead,
	}
}

func (p *Pipeline) handleCheck(ctx context.Context, j *queue.Job) error {
	var pl CheckPayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	return p.CheckSource(ctx, pl.SourceID, pl.RunID)
}

func loadSource(ctx context.Context, pool *pgxpool.Pool, id int64) (Source, error) {
	var s Source
	var u *string
	err := pool.QueryRow(ctx, `
		SELECT id, name, url, city, status, fetch_mode, checks, empty_checks_in_row, status_changed_at, category
		FROM sources WHERE id = $1`, id).
		Scan(&s.ID, &s.Name, &u, &s.City, &s.Status, &s.Mode, &s.Checks, &s.Empty, &s.Changed, &s.Category)
	if u != nil {
		s.URL = *u
	}
	return s, err
}

// checkRecord is one row of source_checks.
type checkRecord struct {
	mode      string
	status    int
	fetchID   *int64
	stats     ResolveStats
	links     int
	err       string
	duration  time.Duration
	checkedAt time.Time
}

// CheckSource loads a source, extracts its events, stores them and updates
// the source. A site that blocks the bot becomes a manual source.
func (p *Pipeline) CheckSource(ctx context.Context, sourceID, runID int64) error {
	src, err := loadSource(ctx, p.Pool, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if src.URL == "" || src.Status == "manual" {
		return nil
	}
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	start := p.now()
	rec := checkRecord{checkedAt: start}

	page, result, mode, fetchErr := p.load(ctx, src)
	rec.mode = string(mode)
	if page != nil {
		rec.status = page.Status
		id, err := storeFetch(ctx, p.Pool, page, fetchErr, start)
		if err != nil {
			return err
		}
		rec.fetchID = &id
	}

	if fetchErr != nil {
		rec.err = fetchErr.Error()
		rec.duration = p.now().Sub(start)
		if err := p.recordCheck(ctx, src, runID, rec); err != nil {
			return err
		}
		if _, err := p.Pool.Exec(ctx, `UPDATE sources SET last_checked_at = $2 WHERE id = $1`, src.ID, start); err != nil {
			return err
		}
		if blocked(fetchErr, page) {
			return p.makeManual(ctx, src, fetchErr)
		}
		return fetchErr
	}

	r := &Resolver{Pool: p.Pool, Rules: RulesFrom(cfg), Now: p.now}
	events := inWindow(result.Events, p.now(), cfg.Days("collect_ahead_days", 30))
	stats, err := r.Resolve(ctx, src, events)
	if err != nil {
		return err
	}
	rec.stats = stats
	rec.links = len(result.Links)
	if _, err := p.discover(ctx, src, result.Links); err != nil {
		p.log().Warn("discovery failed", "source", src.ID, "error", err)
	}
	rec.duration = p.now().Sub(start)
	if err := p.recordCheck(ctx, src, runID, rec); err != nil {
		return err
	}
	// The new events' own pages are read in the same run.
	if _, err := p.enqueueReads(ctx, runID, src.ID, p.readLimit(cfg, "event_pages_per_check", 5)); err != nil {
		p.log().Warn("queueing event reads failed", "source", src.ID, "error", err)
	}
	return p.advance(ctx, src, cfg, stats, mode)
}

// load fetches the page the source's mode asks for, follows feeds the page
// points to, and falls back to the browser when the plain page is an empty
// JavaScript shell.
func (p *Pipeline) load(ctx context.Context, src Source) (*fetch.Page, extract.Result, fetch.Mode, error) {
	get := p.Fetcher.HTTP
	mode := fetch.ModeHTTP
	if src.Mode == "browser" && p.Fetcher.HasBrowser() {
		get, mode = p.Fetcher.Browser, fetch.ModeBrowser
	}
	page, err := get(ctx, src.URL)
	if err != nil {
		return page, extract.Result{}, mode, err
	}
	result := p.extractWithFollow(ctx, page)

	if src.Mode == "auto" && len(result.Events) == 0 && fetch.LooksLikeJSShell(page) && p.Fetcher.HasBrowser() {
		bpage, berr := p.Fetcher.Browser(ctx, src.URL)
		if berr == nil {
			return bpage, p.extractWithFollow(ctx, bpage), fetch.ModeBrowser, nil
		}
		p.log().Warn("browser fallback failed", "source", src.ID, "error", berr)
	}
	return page, result, mode, nil
}

func (p *Pipeline) extractWithFollow(ctx context.Context, page *fetch.Page) extract.Result {
	result := extract.Extract(extract.Page{URL: page.FinalURL, ContentType: page.ContentType, Body: page.Body})
	for i, u := range result.Follow {
		if i >= 3 {
			break
		}
		fp, err := p.Fetcher.HTTP(ctx, u)
		if err != nil {
			p.log().Info("follow failed", "url", u, "error", err)
			continue
		}
		more := extract.Extract(extract.Page{URL: fp.FinalURL, ContentType: fp.ContentType, Body: fp.Body})
		result.Events = append(result.Events, more.Events...)
	}
	return result
}

// inWindow keeps events from yesterday up to the collect-ahead horizon.
func inWindow(events []extract.Event, now time.Time, ahead time.Duration) []extract.Event {
	from, to := now.Add(-24*time.Hour), now.Add(ahead)
	var out []extract.Event
	for _, e := range events {
		end := e.End
		if end.IsZero() {
			end = e.Start
		}
		if end.Before(from) || e.Start.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// blocked reports whether the site refuses the bot, rather than failing
// for a while.
func blocked(err error, page *fetch.Page) bool {
	if errors.Is(err, fetch.ErrRobots) || errors.Is(err, fetch.ErrBlockedHost) {
		return true
	}
	if page != nil && (page.Status == 401 || page.Status == 403) {
		return true
	}
	return false
}

func (p *Pipeline) makeManual(ctx context.Context, src Source, cause error) error {
	note := "Blocks automated access"
	switch {
	case errors.Is(cause, fetch.ErrRobots):
		note = "robots.txt does not allow the bot"
	case errors.Is(cause, fetch.ErrBlockedHost):
		note = "LinkedIn and Instagram are never fetched"
	}
	_, err := p.Pool.Exec(ctx, `
		UPDATE sources SET status = 'manual', status_changed_at = $2, updated_at = $2,
			notes = CASE WHEN notes = '' THEN $3 ELSE notes || '. ' || $3 END
		WHERE id = $1`, src.ID, p.now(), note)
	return err
}

func storeFetch(ctx context.Context, pool *pgxpool.Pool, page *fetch.Page, fetchErr error, at time.Time) (int64, error) {
	var id int64
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
	}
	body, text := page.Body, page.Text
	// Keep stored pages at a sane size.
	if len(body) > 3<<20 {
		body = body[:3<<20]
	}
	if len(text) > 1<<20 {
		text = text[:1<<20]
	}
	err := pool.QueryRow(ctx, `
		INSERT INTO fetches (url, final_url, mode, http_status, content_type, html, visible_text, screenshot, error, duration_ms, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		page.URL, page.FinalURL, string(page.Mode), page.Status, page.ContentType, strings.ToValidUTF8(body, ""),
		strings.ToValidUTF8(text, ""), page.Screenshot, msg, page.Duration.Milliseconds(), at).Scan(&id)
	return id, err
}

func (p *Pipeline) recordCheck(ctx context.Context, src Source, runID int64, rec checkRecord) error {
	var run *int64
	if runID != 0 {
		run = &runID
	}
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO source_checks (run_id, source_id, fetch_id, mode, http_status, events_found, events_kept, events_new,
			people_found, people_new, links_found, error, duration_ms, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		run, src.ID, rec.fetchID, rec.mode, rec.status, rec.stats.EventsFound, rec.stats.EventsKept, rec.stats.EventsNew,
		rec.stats.PeopleFound, rec.stats.PeopleNew, rec.links, rec.err, rec.duration.Milliseconds(), rec.checkedAt)
	return err
}
