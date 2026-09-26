-- The founder rule by title read "Gründer" inside organisation names like
-- "Gründerzentrum" and counted every CEO and managing director. People it
-- marked are looked at again with the rule that reads the role only.
-- Founders the model found on an event page keep their passage and stay.

-- +goose Up

UPDATE people SET fit = 'other' WHERE fit = 'founder' AND fit_evidence = '';

-- Affiliations the old rule called founder, for people no page calls one.
DELETE FROM affiliations af
WHERE af.role = 'founder'
  AND EXISTS (SELECT 1 FROM people p WHERE p.id = af.person_id AND p.fit_evidence = '')
  AND EXISTS (SELECT 1 FROM affiliations o WHERE o.person_id = af.person_id AND o.organisation_id = af.organisation_id AND o.role = 'employee');
UPDATE affiliations af SET role = 'employee'
WHERE af.role = 'founder'
  AND EXISTS (SELECT 1 FROM people p WHERE p.id = af.person_id AND p.fit_evidence = '');

-- The role part of a title is what comes before the first comma or " at ".
UPDATE people SET fit = 'founder'
WHERE fit = 'other' AND fit_evidence = ''
  AND split_part(split_part(headline, ',', 1), ' at ', 1)
      ~* '(^|[^[:alpha:]])(co-?founder|founder|founding partner|mitgründer(in)?|co-?gründer(in)?|gründer(in)?|inhaber(in)?|owner)([^[:alpha:]]|$)'
  AND split_part(split_part(headline, ',', 1), ' at ', 1)
      !~* '(foundation|stiftung|stammtisch|zentrum|center|centre|club|verband|verein|network|netzwerk|allianz|campus|hub|lab)';

-- Meetup's own pages and single Tickettailor events were added as sources.
-- They are retired with a note, the organiser's page comes by itself.
UPDATE sources SET status = 'retired', status_changed_at = now(), next_check_at = now() + interval '10 years',
    notes = CASE WHEN notes = '' THEN 'Not an organiser''s calendar' ELSE notes || '. Not an organiser''s calendar' END
WHERE status <> 'retired' AND (
    url ~* '^https://(www\.)?meetup\.com/(lp|find|topics|cities|apps|login|register|pro|blog|help|about|privacy|terms|start|home|search|members|account)(/|$)'
    OR url ~* '^https://(www\.)?tickettailor\.com/events/[^/]+/[0-9]+');

-- +goose Down

SELECT 1;
