-- The fit rubric: whether a person fits My First Memory, by signals for
-- and against, each with the passage that shows it. See
-- docs/search-strategy.md. person_signals is what the rubric found, done
-- again whenever the person changes or the rubric does. model_signals is
-- what the language model said when it read a page, which the rubric
-- reads too. fit_score is the signals for less those against, and rubric
-- the version of the rubric that scored the person, 0 for not yet.

-- +goose Up

CREATE TABLE person_signals (
    person_id bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    signal    text NOT NULL,
    is_for    boolean NOT NULL,
    label     text NOT NULL,
    passage   text NOT NULL,
    found_in  text NOT NULL DEFAULT '',
    PRIMARY KEY (person_id, signal)
);

CREATE TABLE model_signals (
    person_id bigint NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    signal    text NOT NULL,
    passage   text NOT NULL,
    found_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (person_id, signal, passage)
);

ALTER TABLE people ADD COLUMN fit_score integer NOT NULL DEFAULT 0;
ALTER TABLE people ADD COLUMN rubric integer NOT NULL DEFAULT 0;
CREATE INDEX people_fit_score_idx ON people (fit_score DESC, created_at DESC);
CREATE INDEX people_unscored_idx ON people (rubric);

-- +goose Down

DROP INDEX people_unscored_idx;
DROP INDEX people_fit_score_idx;
ALTER TABLE people DROP COLUMN rubric;
ALTER TABLE people DROP COLUMN fit_score;
DROP TABLE model_signals;
DROP TABLE person_signals;
