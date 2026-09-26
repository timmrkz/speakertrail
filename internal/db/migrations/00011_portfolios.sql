-- A portfolio is a source that lists startups, like an accelerator's or a
-- university's page of the teams it backs. A check of a portfolio stores
-- each startup's website. A lookup then follows the website to its imprint,
-- which names who runs the company.

-- +goose Up

ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'portfolio'));

-- When the startup's imprint was looked up, and where it was.
ALTER TABLE organisations ADD COLUMN looked_up_at timestamptz;
ALTER TABLE organisations ADD COLUMN imprint_url text NOT NULL DEFAULT '';

ALTER TABLE source_checks ADD COLUMN startups_found integer NOT NULL DEFAULT 0;
ALTER TABLE source_checks ADD COLUMN startups_new integer NOT NULL DEFAULT 0;

CREATE TABLE startup_lookups (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id          bigint REFERENCES runs (id) ON DELETE SET NULL,
    organisation_id bigint NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    url             text NOT NULL,
    imprint_url     text NOT NULL DEFAULT '',
    people_found    integer NOT NULL DEFAULT 0,
    people_new      integer NOT NULL DEFAULT 0,
    -- note says why nobody was taken, like "a stock company".
    note            text NOT NULL DEFAULT '',
    error           text NOT NULL DEFAULT '',
    duration_ms     integer NOT NULL DEFAULT 0,
    looked_up_at    timestamptz NOT NULL
);
CREATE INDEX startup_lookups_run_idx ON startup_lookups (run_id);
CREATE INDEX startup_lookups_organisation_idx ON startup_lookups (organisation_id);

INSERT INTO settings (key, value, description) VALUES
    ('startups_per_run', '10', 'Startups whose imprint a run looks up, from portfolios'),
    ('portfolio_check_days', '14', 'Days between checks of a portfolio, which changes slowly')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('startups_per_run', 'portfolio_check_days');
DROP TABLE startup_lookups;
ALTER TABLE source_checks DROP COLUMN startups_new;
ALTER TABLE source_checks DROP COLUMN startups_found;
ALTER TABLE organisations DROP COLUMN imprint_url;
ALTER TABLE organisations DROP COLUMN looked_up_at;
DELETE FROM sources WHERE kind = 'portfolio';
ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query'));
