-- The data model from the brief. Entities (people, organisations, events)
-- exist in the world. Sources are places the engine checks to find them.
-- Sightings link the two, which is the provenance scoring follows.
--
-- Enumerations are text columns with CHECK constraints, so adding a value is
-- a one-line migration.

-- +goose Up

CREATE TABLE seeds (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    input        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    result       text NOT NULL DEFAULT ''
);

CREATE TABLE organisations (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            text NOT NULL,
    normalised_name text NOT NULL,
    kind            text NOT NULL DEFAULT 'other' CHECK (kind IN (
                        'company', 'community', 'meetup_group', 'venue', 'university',
                        'accelerator', 'club', 'public_body', 'other')),
    city            text NOT NULL DEFAULT '',
    website         text NOT NULL DEFAULT '',
    website_domain  text NOT NULL DEFAULT '',
    notes           text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX organisations_normalised_name_idx ON organisations (normalised_name);
CREATE INDEX organisations_website_domain_idx ON organisations (website_domain) WHERE website_domain <> '';

-- People hold names, roles and profile links only. No email addresses or
-- phone numbers, ever.
CREATE TABLE people (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    full_name       text NOT NULL,
    normalised_name text NOT NULL,
    known_as        text NOT NULL DEFAULT '',
    city            text NOT NULL DEFAULT '',
    fit             text NOT NULL DEFAULT 'other' CHECK (fit IN (
                        'founder', 'athlete', 'maker', 'creator', 'other')),
    podcast_status  text NOT NULL DEFAULT 'new' CHECK (podcast_status IN (
                        'new', 'kept', 'skipped', 'known', 'contacted', 'replied',
                        'call', 'yes', 'recorded', 'declined')),
    notes           text NOT NULL DEFAULT '',
    status_changed_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX people_normalised_name_idx ON people (normalised_name);
CREATE INDEX people_skipped_idx ON people (status_changed_at) WHERE podcast_status = 'skipped';

CREATE TABLE events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title         text NOT NULL,
    starts_at     timestamptz NOT NULL,
    ends_at       timestamptz,
    venue_id      bigint REFERENCES organisations (id) ON DELETE SET NULL,
    address       text NOT NULL DEFAULT '',
    city          text NOT NULL DEFAULT '',
    format        text NOT NULL DEFAULT 'in_person' CHECK (format IN ('in_person', 'online', 'hybrid')),
    type          text NOT NULL DEFAULT 'other' CHECK (type IN (
                      'pitch', 'talk', 'panel', 'meetup', 'workshop', 'conference', 'sport', 'other')),
    canonical_url text NOT NULL DEFAULT '',
    price         text NOT NULL DEFAULT '',
    fit           text CHECK (fit IN ('kept', 'dropped')),
    fit_reason    text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at IS NULL OR ends_at >= starts_at)
);
CREATE UNIQUE INDEX events_canonical_url_idx ON events (canonical_url) WHERE canonical_url <> '';
CREATE INDEX events_starts_at_idx ON events (starts_at);

CREATE TABLE sources (
    id                        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                      text NOT NULL DEFAULT '',
    kind                      text NOT NULL CHECK (kind IN (
                                  'listing', 'calendar_luma', 'calendar_meetup', 'calendar_eventbrite',
                                  'calendar_ical', 'organiser_page', 'profile_page', 'newsletter',
                                  'search_query')),
    url                       text,
    query                     text,
    category                  text NOT NULL DEFAULT '',
    city                      text NOT NULL DEFAULT '',
    notes                     text NOT NULL DEFAULT '',
    owner_person_id           bigint REFERENCES people (id) ON DELETE SET NULL,
    owner_organisation_id     bigint REFERENCES organisations (id) ON DELETE SET NULL,
    status                    text NOT NULL DEFAULT 'candidate' CHECK (status IN (
                                  'candidate', 'probation', 'active', 'retired', 'manual')),
    discovered_from_seed_id   bigint REFERENCES seeds (id) ON DELETE SET NULL,
    discovered_from_source_id bigint REFERENCES sources (id) ON DELETE SET NULL,
    discovered_from_person_id bigint REFERENCES people (id) ON DELETE SET NULL,
    discovered_from_organisation_id bigint REFERENCES organisations (id) ON DELETE SET NULL,
    discovered_from_event_id  bigint REFERENCES events (id) ON DELETE SET NULL,
    fetch_mode                text NOT NULL DEFAULT 'auto' CHECK (fetch_mode IN ('auto', 'http', 'browser')),
    check_every               interval,
    last_checked_at           timestamptz,
    next_check_at             timestamptz,
    status_changed_at         timestamptz NOT NULL DEFAULT now(),
    checks                    integer NOT NULL DEFAULT 0,
    empty_checks_in_row       integer NOT NULL DEFAULT 0,
    points                    numeric NOT NULL DEFAULT 0,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'search_query') = (query IS NOT NULL AND url IS NULL)),
    CHECK (kind = 'search_query' OR url IS NOT NULL),
    CHECK (owner_person_id IS NULL OR owner_organisation_id IS NULL)
);
CREATE UNIQUE INDEX sources_url_idx ON sources (url) WHERE url IS NOT NULL;
CREATE UNIQUE INDEX sources_query_idx ON sources (lower(query)) WHERE query IS NOT NULL;
CREATE INDEX sources_due_idx ON sources (status, next_check_at);

CREATE TABLE profiles (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    person_id       bigint REFERENCES people (id) ON DELETE CASCADE,
    organisation_id bigint REFERENCES organisations (id) ON DELETE CASCADE,
    platform        text NOT NULL CHECK (platform IN (
                        'linkedin', 'instagram', 'youtube', 'tiktok', 'x', 'website',
                        'luma', 'meetup', 'podcast', 'other')),
    url             text NOT NULL,
    handle          text NOT NULL DEFAULT '',
    headline        text NOT NULL DEFAULT '',
    review          text NOT NULL DEFAULT 'open' CHECK (review IN ('open', 'confirmed', 'rejected')),
    found_via       text NOT NULL DEFAULT '',
    source_id       bigint REFERENCES sources (id) ON DELETE SET NULL,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((person_id IS NULL) <> (organisation_id IS NULL))
);
CREATE UNIQUE INDEX profiles_platform_url_idx ON profiles (platform, url);
CREATE INDEX profiles_person_idx ON profiles (person_id);
CREATE INDEX profiles_organisation_idx ON profiles (organisation_id);

CREATE TABLE sightings (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id       bigint NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    checked_at      timestamptz NOT NULL,
    event_id        bigint REFERENCES events (id) ON DELETE CASCADE,
    person_id       bigint REFERENCES people (id) ON DELETE CASCADE,
    organisation_id bigint REFERENCES organisations (id) ON DELETE CASCADE,
    profile_id      bigint REFERENCES profiles (id) ON DELETE CASCADE,
    is_new          boolean NOT NULL,
    CHECK (num_nonnulls(event_id, person_id, organisation_id, profile_id) = 1)
);
CREATE INDEX sightings_source_idx ON sightings (source_id, checked_at);
CREATE INDEX sightings_event_idx ON sightings (event_id) WHERE event_id IS NOT NULL;
CREATE INDEX sightings_person_idx ON sightings (person_id) WHERE person_id IS NOT NULL;
CREATE INDEX sightings_organisation_idx ON sightings (organisation_id) WHERE organisation_id IS NOT NULL;
CREATE INDEX sightings_profile_idx ON sightings (profile_id) WHERE profile_id IS NOT NULL;

CREATE TABLE appearances (
    person_id bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    event_id  bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    role      text NOT NULL CHECK (role IN ('speaker', 'panelist', 'pitch', 'host', 'moderator')),
    PRIMARY KEY (person_id, event_id, role)
);
CREATE INDEX appearances_event_idx ON appearances (event_id);

CREATE TABLE organisings (
    organisation_id bigint NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    event_id        bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('organiser', 'co_organiser', 'sponsor', 'venue', 'partner')),
    PRIMARY KEY (organisation_id, event_id, role)
);
CREATE INDEX organisings_event_idx ON organisings (event_id);

CREATE TABLE affiliations (
    person_id       bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    organisation_id bigint NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('founder', 'employee', 'organiser', 'member')),
    is_current      boolean NOT NULL DEFAULT true,
    PRIMARY KEY (person_id, organisation_id, role)
);
CREATE INDEX affiliations_organisation_idx ON affiliations (organisation_id);

-- One post on one platform in one slot. Monday and Thursday are both
-- posts, told apart by their date.
CREATE TABLE posts (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    platform      text NOT NULL CHECK (platform IN ('linkedin', 'instagram')),
    post_date     date NOT NULL,
    status        text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted')),
    url           text NOT NULL DEFAULT '',
    body          text NOT NULL DEFAULT '',
    first_comment text NOT NULL DEFAULT '',
    posted_at     timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (platform, post_date)
);

CREATE TABLE post_events (
    post_id  bigint NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    event_id bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, event_id)
);

-- Everything that happened between Tim and a person. Featured and tagged
-- people of a post are outcomes too, so there is one place to score from.
CREATE TABLE outcomes (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    person_id   bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN (
                    'featured', 'tagged', 'reacted', 'shared', 'messaged', 'replied', 'yes', 'declined')),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    platform    text NOT NULL DEFAULT 'linkedin' CHECK (platform IN ('linkedin', 'instagram', 'other')),
    post_id     bigint REFERENCES posts (id) ON DELETE SET NULL,
    profile_id  bigint REFERENCES profiles (id) ON DELETE SET NULL,
    notes       text NOT NULL DEFAULT ''
);
CREATE INDEX outcomes_person_idx ON outcomes (person_id, occurred_at);
CREATE INDEX outcomes_post_idx ON outcomes (post_id) WHERE post_id IS NOT NULL;

-- +goose Down

DROP TABLE outcomes;
DROP TABLE post_events;
DROP TABLE posts;
DROP TABLE affiliations;
DROP TABLE organisings;
DROP TABLE appearances;
DROP TABLE sightings;
DROP TABLE profiles;
DROP TABLE sources;
DROP TABLE events;
DROP TABLE people;
DROP TABLE organisations;
DROP TABLE seeds;
