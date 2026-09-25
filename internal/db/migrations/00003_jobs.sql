-- The job queue. Kind plus key is unique, so the same work is never queued
-- twice. Workers claim jobs with FOR UPDATE SKIP LOCKED and hold a lease
-- instead of an open transaction, so a crashed worker's job comes back.

-- +goose Up

CREATE TABLE jobs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL,
    key          text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts     integer NOT NULL DEFAULT 0,
    run_after    timestamptz NOT NULL,
    locked_until timestamptz,
    last_error   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (kind, key)
);
CREATE INDEX jobs_ready_idx ON jobs (run_after, id) WHERE status = 'queued';
CREATE INDEX jobs_lease_idx ON jobs (locked_until) WHERE status = 'running';

-- One row per website. A request reserves the next free slot, which keeps
-- every website at one request per interval across all workers.
CREATE TABLE site_slots (
    host            text PRIMARY KEY,
    next_request_at timestamptz NOT NULL
);

-- +goose Down

DROP TABLE site_slots;
DROP TABLE jobs;
