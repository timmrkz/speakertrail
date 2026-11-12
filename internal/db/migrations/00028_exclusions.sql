-- An exclusion is a word or phrase, like "Online Marketing". A profile
-- search leaves out anyone whose headline or roles contain one, and People
-- hides those found before, unless Tim connected with them. Removing an
-- exclusion brings them back. Tim asked for them on 10 November 2026,
-- because online and LinkedIn marketing experts made up half of what some
-- searches brought. How a phrase matches is rubric.Excluded.

-- +goose Up

CREATE TABLE exclusions (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    phrase     text NOT NULL CHECK (phrase <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX exclusions_phrase_idx ON exclusions (lower(phrase));

INSERT INTO exclusions (phrase) VALUES
    ('Online Marketing'), ('Social Media Marketing'), ('Performance Marketing'),
    ('Personal Branding'), ('LinkedIn'), ('Sichtbarkeit');

-- +goose Down

DROP TABLE exclusions;
