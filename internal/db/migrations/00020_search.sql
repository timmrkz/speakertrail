-- Searches find people where no list does: "BJJ Gym Köln", "Life Coach
-- Düsseldorf". Several search providers stand behind one interface, each
-- with a monthly budget in Settings, and the engine stops calling one once
-- its budget is spent. search_calls counts every call, also a failed one,
-- because a provider may bill it.

-- +goose Up

CREATE TABLE search_calls (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider  text NOT NULL,
    query     text NOT NULL,
    results   integer NOT NULL DEFAULT 0,
    error     text NOT NULL DEFAULT '',
    called_at timestamptz NOT NULL
);
CREATE INDEX search_calls_month_idx ON search_calls (provider, called_at);

UPDATE settings SET description = 'Tavily searches per month. Its free plan has 1,000'
WHERE key = 'tavily_monthly_searches';
INSERT INTO settings (key, value, description) VALUES
    ('brave_monthly_searches', '1000', 'Brave searches per month. Its monthly credit covers about 1,000, more are billed')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key = 'brave_monthly_searches';
DROP TABLE search_calls;
