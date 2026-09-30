-- Exa is a third search provider. Its free plan needs no card and gives
-- $10 of searches a month, about 1,400 at its standard price. A budget of
-- 1,000 stays inside it.

-- +goose Up

INSERT INTO settings (key, value, description) VALUES
    ('exa_monthly_searches', '1000', 'Exa searches per month. Its free $10 a month covers about 1,400')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key = 'exa_monthly_searches';
