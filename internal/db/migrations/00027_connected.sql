-- A person is connected or not, nothing else. Tim connects with someone
-- on LinkedIn and marks them here. Whoever he does not connect with simply
-- stays open, so skipping is gone. Kept people become connected, skipped
-- people open again. The later steps of the funnel, contacted to recorded,
-- all count as connected. Decided with Tim on 10 November 2026.

-- +goose Up

ALTER TABLE people DROP CONSTRAINT people_podcast_status_check;
UPDATE people SET podcast_status = 'connected' WHERE podcast_status = 'kept';
UPDATE people SET podcast_status = 'new' WHERE podcast_status = 'skipped';
DROP INDEX IF EXISTS people_skipped_idx;
ALTER TABLE people ADD CONSTRAINT people_podcast_status_check CHECK (podcast_status IN (
    'new', 'connected', 'known', 'contacted', 'replied', 'call', 'yes', 'recorded', 'declined'));

DELETE FROM fit_decisions WHERE decision = 'skipped';
ALTER TABLE fit_decisions DROP CONSTRAINT fit_decisions_decision_check;
UPDATE fit_decisions SET decision = 'connected' WHERE decision = 'kept';
ALTER TABLE fit_decisions ADD CONSTRAINT fit_decisions_decision_check CHECK (decision = 'connected');

DELETE FROM settings WHERE key = 'skipped_person_retention_days';

-- +goose Down

INSERT INTO settings (key, value, description) VALUES
    ('skipped_person_retention_days', '90', 'Days before skipped people are deleted')
ON CONFLICT (key) DO NOTHING;
ALTER TABLE fit_decisions DROP CONSTRAINT fit_decisions_decision_check;
UPDATE fit_decisions SET decision = 'kept' WHERE decision = 'connected';
ALTER TABLE fit_decisions ADD CONSTRAINT fit_decisions_decision_check CHECK (decision IN ('kept', 'skipped'));
ALTER TABLE people DROP CONSTRAINT people_podcast_status_check;
UPDATE people SET podcast_status = 'kept' WHERE podcast_status = 'connected';
ALTER TABLE people ADD CONSTRAINT people_podcast_status_check CHECK (podcast_status IN (
    'new', 'kept', 'skipped', 'known', 'contacted', 'replied', 'call', 'yes', 'recorded', 'declined'));
CREATE INDEX people_skipped_idx ON people (status_changed_at) WHERE podcast_status = 'skipped';
