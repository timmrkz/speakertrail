-- Tim runs the engine by hand on his Mac and would rather start short runs
-- often than wait for long ones. A setting Tim changed keeps his value.

-- +goose Up

INSERT INTO settings (key, value, description) VALUES
    ('sources_per_run', '15', 'Due sources checked per run, the longest overdue first. The rest wait for the next run')
ON CONFLICT (key) DO NOTHING;
UPDATE settings SET value = '5' WHERE key = 'new_candidates_per_run' AND value = '10';
UPDATE settings SET value = '10' WHERE key = 'event_pages_per_run' AND value = '30';
UPDATE settings SET value = '3' WHERE key = 'event_pages_per_check' AND value = '5';

-- +goose Down

DELETE FROM settings WHERE key = 'sources_per_run';
UPDATE settings SET value = '10' WHERE key = 'new_candidates_per_run' AND value = '5';
UPDATE settings SET value = '30' WHERE key = 'event_pages_per_run' AND value = '10';
UPDATE settings SET value = '5' WHERE key = 'event_pages_per_check' AND value = '3';
