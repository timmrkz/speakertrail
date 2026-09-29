-- A source retired by hand kept its old next check and came back in the
-- next run, like meetup.com/lp, a page that does not exist. Retired sources
-- that are due now were retired by hand, because the engine gives the ones
-- it retires a recheck weeks away. They are not checked again until Tim
-- sets them back. Meetup's own pages and single Tickettailor events are
-- retired too, whatever their status.

-- +goose Up

UPDATE sources SET next_check_at = now() + interval '10 years'
WHERE status = 'retired' AND (next_check_at IS NULL OR next_check_at <= now());

UPDATE sources SET status = 'retired', status_changed_at = now(), next_check_at = now() + interval '10 years'
WHERE (next_check_at IS NULL OR next_check_at < now() + interval '1 year') AND (
    url ~* '^https://(www\.)?meetup\.com/(lp|find|topics|cities|apps|login|register|pro|blog|help|about|privacy|terms|start|home|search|members|account)(/|$)'
    OR url ~* '^https://(www\.)?tickettailor\.com/events/[^/]+/[0-9]+');

-- Checks left waiting for a retry end, so no run waits for them.
UPDATE jobs SET status = 'failed', last_error = 'checks are no longer retried within a run', locked_until = NULL, updated_at = now()
WHERE kind IN ('check_source', 'read_event') AND status = 'queued' AND attempts > 0;

-- +goose Down

SELECT 1;
