-- A read is the local language model's look at one event's own page, to
-- find who is on stage. Runs queue reads for upcoming events.

-- +goose Up

-- When the event's page was read. Unread events are read once.
ALTER TABLE events ADD COLUMN people_read_at timestamptz;

-- The passage from the event page that puts the person on stage, so Tim
-- sees why someone is on the list.
ALTER TABLE appearances ADD COLUMN evidence text NOT NULL DEFAULT '';

CREATE TABLE event_reads (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       bigint REFERENCES runs (id) ON DELETE SET NULL,
    event_id     bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    url          text NOT NULL,
    model        text NOT NULL DEFAULT '',
    people_found integer NOT NULL DEFAULT 0,
    people_new   integer NOT NULL DEFAULT 0,
    error        text NOT NULL DEFAULT '',
    duration_ms  integer NOT NULL DEFAULT 0,
    read_at      timestamptz NOT NULL
);
CREATE INDEX event_reads_run_idx ON event_reads (run_id);
CREATE INDEX events_unread_idx ON events (starts_at) WHERE people_read_at IS NULL;

INSERT INTO settings (key, value, description) VALUES
    ('event_pages_per_run', '30', 'Event pages the language model reads per run, for events whose page it has not read yet'),
    ('event_pages_per_check', '5', 'Event pages read right after a source check, for the new events it found')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('event_pages_per_run', 'event_pages_per_check');
DROP TABLE event_reads;
ALTER TABLE appearances DROP COLUMN evidence;
ALTER TABLE events DROP COLUMN people_read_at;
