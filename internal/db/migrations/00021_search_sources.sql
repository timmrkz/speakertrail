-- A search is a source: its results lead to websites like a directory's
-- entries. A run takes only a few due searches, because each brings up to
-- 20 websites to look up, and results change slowly, so a search is
-- checked again after a month.

-- +goose Up

INSERT INTO settings (key, value, description) VALUES
    ('searches_per_run', '3', 'Due searches a run takes. Each brings up to 20 websites to look up'),
    ('search_check_days', '30', 'Days between two runs of the same search')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('searches_per_run', 'search_check_days');
