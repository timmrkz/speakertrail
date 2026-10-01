-- A run says how it ended: it finished, Tim stopped it, or the app
-- stopped under it and it ended when the app started again. A run that
-- has not ended says nothing yet. Each new person remembers the run that
-- found them, so a run can say what it brought.

-- +goose Up

ALTER TABLE runs ADD COLUMN ended text NOT NULL DEFAULT '' CHECK (ended IN ('', 'finished', 'stopped', 'restart'));

-- Earlier runs, by what their dropped jobs say.
UPDATE runs r SET ended = 'restart' WHERE EXISTS (
    SELECT 1 FROM jobs j WHERE j.key LIKE 'run:' || r.id || ':%' AND j.last_error = 'the app stopped while this waited or ran');
UPDATE runs r SET ended = 'stopped' WHERE ended = '' AND EXISTS (
    SELECT 1 FROM jobs j WHERE j.key LIKE 'run:' || r.id || ':%' AND j.last_error = 'stopped by hand');
UPDATE runs SET ended = 'finished' WHERE ended = '' AND finished_at IS NOT NULL;

ALTER TABLE people ADD COLUMN first_run_id bigint REFERENCES runs (id) ON DELETE SET NULL;
CREATE INDEX people_first_run_idx ON people (first_run_id);

-- People found before this change belong to the run that was going when
-- they were added: the last one started before, within three hours.
UPDATE people p SET first_run_id = (
    SELECT r.id FROM runs r
    WHERE r.started_at <= p.created_at AND r.started_at > p.created_at - interval '3 hours'
    ORDER BY r.started_at DESC LIMIT 1)
WHERE first_run_id IS NULL;

-- +goose Down

DROP INDEX people_first_run_idx;
ALTER TABLE people DROP COLUMN first_run_id;
ALTER TABLE runs DROP COLUMN ended;
