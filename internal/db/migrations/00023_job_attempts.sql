-- A job can have its own number of attempts. Jobs of a run get one, so a
-- failing check never keeps a run waiting for a retry an hour later. The
-- source is simply due again in the next run. Empty means the queue's
-- default.

-- +goose Up

ALTER TABLE jobs ADD COLUMN max_attempts integer CHECK (max_attempts > 0);

-- +goose Down

ALTER TABLE jobs DROP COLUMN max_attempts;
