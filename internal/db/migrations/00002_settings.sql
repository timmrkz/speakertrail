-- Every setting is one row Tim can edit in the Settings screen. Values are
-- JSON so a setting can be a number, a duration, a list or a map.

-- +goose Up

CREATE TABLE settings (
    key         text PRIMARY KEY,
    value       jsonb NOT NULL,
    description text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO settings (key, value, description) VALUES
    ('region', '["Köln", "Bonn", "Düsseldorf", "Essen", "Dortmund", "Duisburg", "Bochum", "Aachen", "Münster", "Wuppertal", "Bielefeld", "Gelsenkirchen", "Mönchengladbach", "Krefeld", "Oberhausen", "Hagen", "Hamm", "Mülheim an der Ruhr", "Leverkusen", "Solingen", "Herne", "Neuss", "Paderborn", "Bottrop", "Recklinghausen", "Remscheid", "Bergisch Gladbach", "Moers", "Siegen", "Gütersloh", "Witten", "Iserlohn", "Düren", "Ratingen", "Lünen", "Marl", "Velbert", "Minden", "Viersen", "Troisdorf", "Rheine", "Dorsten", "Castrop-Rauxel", "Arnsberg", "Detmold", "Lüdenscheid", "Bocholt", "Grevenbroich", "Unna", "Dinslaken", "Herford", "Kerpen", "Lippstadt", "Bergheim", "Dormagen", "Gladbeck", "Sankt Augustin", "Wesel", "Hürth", "Siegburg", "Frechen"]',
        'Cities in NRW whose in-person events count'),
    ('collect_ahead_days', '30', 'How far ahead events are collected, in days'),
    ('post_window_days', '7', 'Which events go into a post, in days from the post date'),
    ('active_check_interval_days', '1', 'Days between checks of an active source. 1 means every run'),
    ('probation_weeks', '3', 'Weeks a source stays on probation before it becomes active or retired'),
    ('probation_checks', '3', 'Checks before a candidate becomes active or retired'),
    ('retire_after_empty_checks', '4', 'Empty checks in a row before a source retires'),
    ('retired_recheck_days', '28', 'Days between checks of a retired source'),
    ('new_candidates_per_run', '10', 'Candidate sources checked for the first time per run'),
    ('profile_recheck_days', '90', 'Days before a profile is looked at again'),
    ('scoring_window_weeks', '8', 'How far back points count, in weeks'),
    ('discovery_share', '0.2', 'Share of points passed to the source that discovered another'),
    ('points', '{"event": 1, "person": 1, "reaction": 3, "reply": 5, "yes": 10}', 'Points per result'),
    ('tags_per_post', '3', 'Maximum tagged accounts per post'),
    ('events_per_post', '{"min": 6, "max": 10}', 'Size of a post, grouped by city'),
    ('keep_words', '["founder", "gründer", "pitch", "meetup", "sport", "maker"]', 'First filter that keeps an event'),
    ('drop_words', '["webinar", "training", "sales"]', 'First filter that drops an event'),
    ('skipped_person_retention_days', '90', 'Days before skipped people are deleted'),
    ('snapshot_retention_days', '30', 'Days raw pages and screenshots are kept'),
    ('crawl_time', '"05:00"', 'Time of the nightly run, Europe/Berlin'),
    ('draft_days', '["sunday", "wednesday"]', 'Days the next post is drafted'),
    ('draft_time', '"16:00"', 'Time drafts are made, Europe/Berlin'),
    ('post_days', '["monday", "thursday"]', 'Days posts go out'),
    ('site_request_interval_seconds', '5', 'Minimum seconds between two requests to the same website'),
    ('worker_concurrency', '4', 'Jobs a worker runs at the same time'),
    ('browser_pages', '2', 'Headless browser pages open at the same time'),
    ('tavily_monthly_searches', '1000', 'Web searches per month, discovery and profiles together'),
    ('tavily_profile_share', '0.5', 'Share of the monthly searches used for profile lookups'),
    ('claude_monthly_budget_eur', '4', 'Monthly cap for Claude API calls, in euros'),
    ('voice_guide', '""', 'How Tim writes, used when drafting posts');

-- +goose Down

DROP TABLE settings;
