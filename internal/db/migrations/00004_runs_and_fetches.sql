-- Runs, per-source checks and stored pages. A run is one nightly pass, or a
-- single "check now" from the interface. Every source check records what
-- happened, which drives the health warnings. Fetches keep the raw page, its
-- visible text and a screenshot for 30 days, so failures can be looked into.

-- +goose Up

CREATE TABLE runs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text NOT NULL DEFAULT 'nightly' CHECK (kind IN ('nightly', 'manual')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

CREATE TABLE fetches (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url          text NOT NULL,
    final_url    text NOT NULL DEFAULT '',
    mode         text NOT NULL CHECK (mode IN ('http', 'browser')),
    http_status  integer NOT NULL DEFAULT 0,
    content_type text NOT NULL DEFAULT '',
    html         text NOT NULL DEFAULT '',
    visible_text text NOT NULL DEFAULT '',
    screenshot   bytea,
    error        text NOT NULL DEFAULT '',
    duration_ms  integer NOT NULL DEFAULT 0,
    fetched_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fetches_fetched_at_idx ON fetches (fetched_at);

CREATE TABLE source_checks (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       bigint REFERENCES runs (id) ON DELETE SET NULL,
    source_id    bigint NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    fetch_id     bigint REFERENCES fetches (id) ON DELETE SET NULL,
    mode         text NOT NULL DEFAULT '',
    http_status  integer NOT NULL DEFAULT 0,
    events_found integer NOT NULL DEFAULT 0,
    events_kept  integer NOT NULL DEFAULT 0,
    events_new   integer NOT NULL DEFAULT 0,
    people_found integer NOT NULL DEFAULT 0,
    people_new   integer NOT NULL DEFAULT 0,
    links_found  integer NOT NULL DEFAULT 0,
    error        text NOT NULL DEFAULT '',
    duration_ms  integer NOT NULL DEFAULT 0,
    checked_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX source_checks_source_idx ON source_checks (source_id, checked_at DESC);
CREATE INDEX source_checks_run_idx ON source_checks (run_id);

-- Manual and retired sources from the brief may have no address. They stay
-- in the table so the engine never adds them again.
ALTER TABLE sources DROP CONSTRAINT sources_check1;
ALTER TABLE sources ADD CONSTRAINT sources_url_required
    CHECK (kind = 'search_query' OR url IS NOT NULL OR status IN ('manual', 'retired'));

-- Where a source came from, when it was not another row.
ALTER TABLE sources ADD COLUMN discovered_note text NOT NULL DEFAULT '';

INSERT INTO settings (key, value, description) VALUES
    ('public_calendar', 'false', 'Show the event calendar to everyone without a login'),
    ('public_show_people', 'false', 'Show the names of speakers in the public calendar'),
    ('user_agent_contact', '"https://github.com/timmrkz/speakertrail"', 'Contact address in the bot''s User-Agent');

-- +goose Down

DELETE FROM settings WHERE key IN ('public_calendar', 'public_show_people', 'user_agent_contact');
ALTER TABLE sources DROP COLUMN discovered_note;
ALTER TABLE sources DROP CONSTRAINT sources_url_required;
ALTER TABLE sources ADD CONSTRAINT sources_check1 CHECK (kind = 'search_query' OR url IS NOT NULL);
DROP TABLE source_checks;
DROP TABLE fetches;
DROP TABLE runs;
