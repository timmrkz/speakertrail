-- Tim looks for founders, not for everyone on a stage. A person is a
-- founder when an event page says they founded or run something, and the
-- passage that says so is kept.

-- +goose Up

ALTER TABLE people ADD COLUMN fit_evidence text NOT NULL DEFAULT '';

-- People whose title or affiliation already says so.
UPDATE people p SET fit = 'founder'
WHERE p.fit = 'other' AND (
    p.headline ~* '(founder|gründer|inhaber|geschäftsführ)'
    OR EXISTS (SELECT 1 FROM affiliations af WHERE af.person_id = p.id AND af.role = 'founder'));

-- Upcoming events read before the model looked for founders are read
-- again by the next run.
UPDATE events SET people_read_at = NULL WHERE people_read_at IS NOT NULL AND starts_at >= now();

-- +goose Down

ALTER TABLE people DROP COLUMN fit_evidence;
