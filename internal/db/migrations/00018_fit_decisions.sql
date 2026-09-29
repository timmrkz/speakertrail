-- Tim keeps a person, to contact them, or skips them. Each decision
-- remembers the rubric's signals the person had then, so the counts of
-- keeps and skips per signal show which signals predict a keep, and which
-- mislead. A decision outlives the person: skipped people are deleted
-- after a while, and their skip still counts. It names no one.

-- +goose Up

CREATE TABLE fit_decisions (
    person_id  bigint REFERENCES people (id) ON DELETE SET NULL,
    decision   text NOT NULL CHECK (decision IN ('kept', 'skipped')),
    signals    text[] NOT NULL DEFAULT '{}',
    fit_score  integer NOT NULL DEFAULT 0,
    rubric     integer NOT NULL DEFAULT 0,
    decided_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX fit_decisions_person_idx ON fit_decisions (person_id) WHERE person_id IS NOT NULL;

-- +goose Down

DROP TABLE fit_decisions;
