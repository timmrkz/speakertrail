-- A directory is a source that lists businesses run by people rather than
-- startups, like a list of gyms or a coaching association's members. It
-- works like a portfolio: each entry leads to its website, its imprint and
-- its about page. Directories are national, so only entries in NRW count,
-- by the postcode the list or the imprint gives.

-- +goose Up

ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'portfolio', 'directory'));

-- +goose Down

UPDATE sources SET kind = 'portfolio' WHERE kind = 'directory';
ALTER TABLE sources DROP CONSTRAINT sources_kind_check;
ALTER TABLE sources ADD CONSTRAINT sources_kind_check CHECK (kind IN (
    'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
    'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
    'search_query', 'portfolio'));
