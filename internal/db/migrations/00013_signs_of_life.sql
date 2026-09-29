-- A portfolio says a startup existed once, not that it still works. A
-- lookup records signs of life: whether the website answers or is parked,
-- whether the imprint says the company is being wound up, and the newest
-- date on the site. Startups are looked up again every 90 days.

-- +goose Up

ALTER TABLE organisations ADD COLUMN activity text NOT NULL DEFAULT '' CHECK (activity IN (
    '', 'active', 'unknown', 'quiet', 'dissolved', 'gone'));
-- The newest date the website shows, from its sitemap or its copyright.
ALTER TABLE organisations ADD COLUMN last_sign_at timestamptz;
ALTER TABLE organisations ADD COLUMN activity_note text NOT NULL DEFAULT '';

INSERT INTO settings (key, value, description) VALUES
    ('relookup_days', '90', 'Days after which a startup is looked up again, to see whether it is still active'),
    ('quiet_after_days', '365', 'Days without any sign of life after which a startup counts as quiet')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM settings WHERE key IN ('relookup_days', 'quiet_after_days');
ALTER TABLE organisations DROP COLUMN activity_note;
ALTER TABLE organisations DROP COLUMN last_sign_at;
ALTER TABLE organisations DROP COLUMN activity;
