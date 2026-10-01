-- Guests now come from events on other stages too: breathwork, yoga,
-- open mats, readings and coaches' workshops. "training" as a drop word
-- threw out exactly those, like "Atem-Training" or "BJJ Training". It was
-- meant for corporate sales trainings, which "sales" still drops. A drop
-- list Tim changed keeps his value.

-- +goose Up

UPDATE settings SET value = '["webinar", "sales"]'
WHERE key = 'drop_words' AND value = '["webinar", "training", "sales"]'::jsonb;

-- +goose Down

UPDATE settings SET value = '["webinar", "training", "sales"]'
WHERE key = 'drop_words' AND value = '["webinar", "sales"]'::jsonb;
