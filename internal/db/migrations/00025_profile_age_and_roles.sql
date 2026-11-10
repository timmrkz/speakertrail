-- What a LinkedIn profile in Exa's index says about a person, for the fit
-- rubric. born_about is the year they were likely born, guessed from the
-- years of their education and first jobs, and age_reason the step that
-- shows it, like "Bachelor from 2018". current_roles are the jobs they
-- hold now, like "Inhaberin, Studio Beispiel". The rest of the profile's
-- jobs and education is never stored. See docs/search-strategy.md.

-- +goose Up

ALTER TABLE profiles
    ADD COLUMN born_about    smallint,
    ADD COLUMN age_reason    text NOT NULL DEFAULT '',
    ADD COLUMN current_roles text NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE profiles DROP COLUMN born_about, DROP COLUMN age_reason, DROP COLUMN current_roles;
