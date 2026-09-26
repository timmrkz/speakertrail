package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/llm"
	"github.com/timmrkz/speakertrail/internal/queue"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// KindReadEvent is a read: the language model looks at one event's own
// page for the people on stage.
const KindReadEvent = "read_event"

// ReadPayload is the payload of a read_event job.
type ReadPayload struct {
	EventID int64 `json:"event_id"`
	RunID   int64 `json:"run_id"`
}

// Reader finds the people on stage in a page's text. *llm.Client is one.
type Reader interface {
	People(ctx context.Context, title, text string) (llm.PeopleResult, error)
}

// unreadEvents are upcoming kept events with a page of their own that the
// model has not read yet. A page that is a source itself is a listing, not
// one event's page.
const unreadEvents = `
	SELECT e.id FROM events e
	WHERE e.people_read_at IS NULL AND e.fit = 'kept' AND e.format <> 'online'
	  AND e.starts_at >= $1 AND e.canonical_url <> '' AND e.canonical_url NOT LIKE '%.ics'
	  AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.url = e.canonical_url)`

// enqueueReads queues reads of unread events for a run, soonest first. With
// a source it only takes events that source showed.
func (p *Pipeline) enqueueReads(ctx context.Context, runID, sourceID int64, limit int) (int, error) {
	if p.Reader == nil || limit <= 0 {
		return 0, nil
	}
	sql := unreadEvents
	args := []any{p.now(), limit}
	if sourceID != 0 {
		sql += ` AND EXISTS (SELECT 1 FROM sightings si WHERE si.event_id = e.id AND si.source_id = $3)`
		args = append(args, sourceID)
	}
	rows, err := p.Pool.Query(ctx, sql+` ORDER BY e.starts_at, e.id LIMIT $2`, args...)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := p.Queue.Enqueue(ctx, queue.NewJob{
			Kind: KindReadEvent, Key: fmt.Sprintf("run:%d:read:%d", runID, id),
			Payload: ReadPayload{EventID: id, RunID: runID},
		}); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (p *Pipeline) handleRead(ctx context.Context, j *queue.Job) error {
	var pl ReadPayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	return p.ReadEvent(ctx, pl.EventID, pl.RunID)
}

// ReadEvent loads one event's page, asks the model who is on stage and
// stores those people with the passage that shows it. Each event is read
// once. Without a model it does nothing, and the event waits for a run
// that has one.
func (p *Pipeline) ReadEvent(ctx context.Context, eventID, runID int64) error {
	if p.Reader == nil {
		return nil
	}
	var title, url, city string
	var sourceID *int64
	var readAt *time.Time
	err := p.Pool.QueryRow(ctx, `
		SELECT e.title, e.canonical_url, e.city, e.people_read_at,
			(SELECT si.source_id FROM sightings si WHERE si.event_id = e.id ORDER BY si.id LIMIT 1)
		FROM events e WHERE e.id = $1`, eventID).Scan(&title, &url, &city, &readAt, &sourceID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (readAt != nil || url == "" || sourceID == nil)) {
		return nil
	}
	if err != nil {
		return err
	}
	start := p.now()
	rec := readRecord{runID: runID, eventID: eventID, url: url, at: start}
	if c, ok := p.Reader.(*llm.Client); ok {
		rec.model = c.Model
	}

	page, err := p.Fetcher.HTTP(ctx, url)
	if err == nil && page.IsHTML() && fetch.LooksLikeJSShell(page) && p.Fetcher.HasBrowser() {
		page, err = p.Fetcher.Browser(ctx, url)
	}
	if err != nil {
		// A page that refuses the bot is not asked again. Anything else is
		// retried by the queue.
		if blocked(err, page) {
			rec.err = err.Error()
			return p.finishRead(ctx, rec, nil, 0)
		}
		return err
	}

	if until := p.modelPause(); !until.IsZero() {
		// The model failed a moment ago. The event waits for a later run.
		return nil
	}
	found, err := p.Reader.People(ctx, title, page.Text)
	if llm.Unavailable(err) {
		// The model cannot answer now. The event stays unread for a later
		// run, the failure is recorded, and reads pause for a while so the
		// rest of the run does not wait on a broken model page by page.
		p.pauseModel()
		p.log().Warn("the language model failed, event pages wait for a later run", "error", err)
		rec.err = err.Error()
		rec.duration = p.now().Sub(start)
		return p.recordRead(ctx, p.Pool, rec)
	}
	if err != nil {
		return err
	}
	rec.duration = p.now().Sub(start)
	return p.finishRead(ctx, rec, found.People, *sourceID)
}

type readRecord struct {
	runID, eventID int64
	url, model     string
	err            string
	duration       time.Duration
	at             time.Time
	found, isNew   int
}

// finishRead stores the people of one read, marks the event read and
// records the read, in one short transaction.
func (p *Pipeline) finishRead(ctx context.Context, rec readRecord, people []llm.Person, sourceID int64) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var city string
	if err := tx.QueryRow(ctx, `SELECT city FROM events WHERE id = $1 FOR UPDATE`, rec.eventID).Scan(&city); err != nil {
		return err
	}
	r := &Resolver{Pool: p.Pool, Now: p.now}
	for _, person := range people {
		pid, isNew, err := r.resolvePerson(ctx, tx, rec.eventID, city,
			extract.Person{Name: person.Name, Role: person.Role, Affiliation: person.Affiliation}, rec.url, rec.at)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE appearances SET evidence = $3 WHERE person_id = $1 AND event_id = $2 AND evidence = ''`,
			pid, rec.eventID, person.Evidence); err != nil {
			return err
		}
		if err := sight(ctx, tx, sourceID, rec.at, "person_id", pid, isNew); err != nil {
			return err
		}
		rec.found++
		if isNew {
			rec.isNew++
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE events SET people_read_at = $2 WHERE id = $1`, rec.eventID, rec.at); err != nil {
		return err
	}
	if err := p.recordRead(ctx, tx, rec); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// recordRead writes one row of event_reads, also for a read that failed.
func (p *Pipeline) recordRead(ctx context.Context, db execer, rec readRecord) error {
	var run *int64
	if rec.runID != 0 {
		run = &rec.runID
	}
	_, err := db.Exec(ctx, `
		INSERT INTO event_reads (run_id, event_id, url, model, people_found, people_new, error, duration_ms, read_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		run, rec.eventID, rec.url, rec.model, rec.found, rec.isNew, rec.err, rec.duration.Milliseconds(), rec.at)
	return err
}

// modelPauseFor is how long reads rest after the model failed.
const modelPauseFor = 10 * time.Minute

// pauseModel stops reads for a while. Workers share it, so it is guarded.
func (p *Pipeline) pauseModel() {
	p.mu.Lock()
	p.modelDownUntil = p.now().Add(modelPauseFor)
	p.mu.Unlock()
}

// modelPause returns until when reads rest, or zero when they do not.
func (p *Pipeline) modelPause() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.now().Before(p.modelDownUntil) {
		return p.modelDownUntil
	}
	return time.Time{}
}

// readLimit is a setting that caps reads, 0 without a model.
func (p *Pipeline) readLimit(cfg settings.Settings, key string, def int) int {
	if p.Reader == nil {
		return 0
	}
	return cfg.Int(key, def)
}
