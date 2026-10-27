-- A post search is a source that asks Exa for LinkedIn posts, like "Ein
-- Jahr selbstständig #köln". Everyone who wrote one is active there by
-- definition. Each post's author is looked up in Exa's index, never on
-- LinkedIn itself, and only authors who live in NRW are kept. The post
-- that brought a person is kept too, with its date and its first line,
-- as the reason they appear. See docs/search-strategy.md.

-- +goose Up

ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'post_search', 'portfolio', 'directory'));
ALTER TABLE sources DROP CONSTRAINT sources_check;
ALTER TABLE sources ADD CONSTRAINT sources_check CHECK (
    (kind IN ('search_query', 'post_search')) = (query IS NOT NULL AND url IS NULL));
ALTER TABLE sources DROP CONSTRAINT sources_url_required;
ALTER TABLE sources ADD CONSTRAINT sources_url_required CHECK (
    kind IN ('search_query', 'post_search') OR url IS NOT NULL OR status IN ('manual', 'retired'));

CREATE TABLE person_posts (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    person_id    bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    url          text NOT NULL UNIQUE,
    title        text NOT NULL DEFAULT '',
    published_at timestamptz,
    source_id    bigint REFERENCES sources (id) ON DELETE SET NULL,
    found_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX person_posts_person_idx ON person_posts (person_id, published_at DESC);

INSERT INTO settings (key, value, description) VALUES
    ('post_searches_per_run', '2', 'Due post searches a run takes. Each brings up to 10 posts and looks up their authors'),
    ('post_search_check_days', '14', 'Days between two runs of the same post search'),
    ('post_search_days', '90', 'How far back a post search looks, in days')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('post_searches_per_run', 'post_search_check_days', 'post_search_days');
DROP TABLE person_posts;
DELETE FROM sources WHERE kind = 'post_search';
ALTER TABLE sources DROP CONSTRAINT sources_url_required;
ALTER TABLE sources ADD CONSTRAINT sources_url_required CHECK (
    kind = 'search_query' OR url IS NOT NULL OR status IN ('manual', 'retired'));
ALTER TABLE sources DROP CONSTRAINT sources_check;
ALTER TABLE sources ADD CONSTRAINT sources_check CHECK (
    (kind = 'search_query') = (query IS NOT NULL AND url IS NULL));
ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'portfolio', 'directory'));
