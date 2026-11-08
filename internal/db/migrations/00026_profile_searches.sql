-- A profile search is a source that asks Exa for LinkedIn profiles of a
-- kind of person in a place, like "Personal Trainer Düsseldorf". Only
-- people who live in NRW, were active on LinkedIn lately and run their
-- own business are kept. It is the one way runs find people now, see
-- docs/search-strategy.md. last_active_at is the newest post, comment or
-- reaction a profile shows.

-- +goose Up

ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'post_search', 'profile_search', 'portfolio', 'directory'));
ALTER TABLE sources DROP CONSTRAINT sources_check;
ALTER TABLE sources ADD CONSTRAINT sources_check CHECK (
    (kind IN ('search_query', 'post_search', 'profile_search')) = (query IS NOT NULL AND url IS NULL));
ALTER TABLE sources DROP CONSTRAINT sources_url_required;
ALTER TABLE sources ADD CONSTRAINT sources_url_required CHECK (
    kind IN ('search_query', 'post_search', 'profile_search') OR url IS NOT NULL OR status IN ('manual', 'retired'));

-- The same words can be a website search and a profile search.
DROP INDEX sources_query_idx;
CREATE UNIQUE INDEX sources_query_idx ON sources (kind, lower(query)) WHERE query IS NOT NULL;

ALTER TABLE profiles ADD COLUMN last_active_at timestamptz;

INSERT INTO settings (key, value, description) VALUES
    ('profile_searches_per_run', '10', 'Due profile searches a run takes. Each is one call to Exa'),
    ('profile_search_results', '25', 'Profiles one profile search asks for'),
    ('profile_search_check_days', '30', 'Days between two runs of the same profile search'),
    ('profile_active_days', '183', 'Days within which a person must have been active on LinkedIn to be kept'),
    ('profile_searches_only', 'true', 'Runs take only profile searches. Events, portfolios, directories, website and post searches are paused')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('profile_searches_per_run', 'profile_search_results', 'profile_search_check_days', 'profile_active_days', 'profile_searches_only');
ALTER TABLE profiles DROP COLUMN last_active_at;
DELETE FROM sources WHERE kind = 'profile_search';
DROP INDEX sources_query_idx;
CREATE UNIQUE INDEX sources_query_idx ON sources (lower(query)) WHERE query IS NOT NULL;
ALTER TABLE sources DROP CONSTRAINT sources_url_required;
ALTER TABLE sources ADD CONSTRAINT sources_url_required CHECK (
    kind IN ('search_query', 'post_search') OR url IS NOT NULL OR status IN ('manual', 'retired'));
ALTER TABLE sources DROP CONSTRAINT sources_check;
ALTER TABLE sources ADD CONSTRAINT sources_check CHECK (
    (kind IN ('search_query', 'post_search')) = (query IS NOT NULL AND url IS NULL));
ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'post_search', 'portfolio', 'directory'));
