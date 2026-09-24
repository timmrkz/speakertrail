-- Columns the extraction and the interface need.

-- +goose Up

-- Tim's keep or drop decision wins over the automatic fit rules.
ALTER TABLE events ADD COLUMN fit_manual boolean NOT NULL DEFAULT false;
ALTER TABLE events ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN normalised_title text NOT NULL DEFAULT '';
CREATE INDEX events_title_start_idx ON events (normalised_title, starts_at);

-- The short line that says who someone is, as the event page put it.
ALTER TABLE people ADD COLUMN headline text NOT NULL DEFAULT '';

CREATE INDEX appearances_person_idx ON appearances (person_id);
CREATE INDEX affiliations_person_idx ON affiliations (person_id);

-- +goose Down

DROP INDEX affiliations_person_idx;
DROP INDEX appearances_person_idx;
ALTER TABLE people DROP COLUMN headline;
DROP INDEX events_title_start_idx;
ALTER TABLE events DROP COLUMN normalised_title;
ALTER TABLE events DROP COLUMN description;
ALTER TABLE events DROP COLUMN fit_manual;
