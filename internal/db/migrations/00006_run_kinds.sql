-- A run is one pass over the due sources, nightly or started by hand.
-- Check now on one source gets its own kind, so the two are told apart.

-- +goose Up

ALTER TABLE runs DROP CONSTRAINT runs_kind_check;
UPDATE runs SET kind = 'check' WHERE kind = 'manual';
ALTER TABLE runs ADD CONSTRAINT runs_kind_check CHECK (kind IN ('nightly', 'manual', 'check'));

-- +goose Down

UPDATE runs SET kind = 'manual' WHERE kind = 'check';
ALTER TABLE runs DROP CONSTRAINT runs_kind_check;
ALTER TABLE runs ADD CONSTRAINT runs_kind_check CHECK (kind IN ('nightly', 'manual'));
