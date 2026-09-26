# HTTP API

The web interface talks to `speakertrail serve` through this JSON API. All times are RFC 3339 strings in UTC. All private endpoints answer `401 {"error": "..."}` without a valid session cookie. Errors are always `{"error": "message"}` with a fitting status code.

## Public

The front page is always the calendar. It is closed to visitors until the setting `public_calendar` is true. While it is closed, `config` answers with `public_calendar` false and no cities, `events` answers 404, and the page says the calendar opens soon. Tim, logged in, gets `owner` true and sees the calendar before it opens.

`GET /api/public/config`

```json
{ "public_calendar": true, "owner": false, "show_people": false, "cities": ["Köln", "Düsseldorf"] }
```

`cities` lists the cities with upcoming kept events, most events first.

`GET /api/public/events?from=2026-09-25&to=2026-10-25&city=Köln&type=pitch`

All parameters are optional. `from` defaults to today, `to` to `from` plus the setting `collect_ahead_days`. Only kept, in-person or hybrid events are returned.

```json
{
  "events": [
    {
      "id": 12,
      "title": "Rheinland Pitch #129",
      "starts_at": "2026-09-28T16:00:00Z",
      "ends_at": null,
      "city": "Köln",
      "venue": "Startplatz",
      "address": "Im Mediapark 5, 50670 Köln",
      "format": "in_person",
      "type": "pitch",
      "url": "https://www.startplatz.de/event/rheinland-pitch-koeln-2026-09-28/",
      "price": "",
      "organisers": ["Rheinland Pitch", "Startplatz"],
      "people": [{ "name": "Lea Beispiel", "role": "pitch", "affiliation": "Beispiel GmbH" }]
    }
  ]
}
```

`people` stays empty unless the setting `public_show_people` is true.

## Login

`POST /api/login` with `{"password": "..."}` answers 204 and sets the session cookie, or 401.

`POST /api/logout` answers 204 and clears the cookie.

`GET /api/me` answers `{"logged_in": true}` or `{"logged_in": false}`.

## Private

`GET /api/stats`

```json
{
  "totals": {
    "people": 312, "organisations": 140, "profiles": 88,
    "events_upcoming": 96, "events_kept_upcoming": 61,
    "sources": { "active": 16, "probation": 20, "candidate": 58, "retired": 7, "manual": 5 }
  },
  "last_7_days": { "people": 41, "events": 23, "organisations": 12, "sources": 9 },
  "weekly": [ { "week_start": "2026-08-03", "people": 0, "events": 0, "sources": 0 } ],
  "cities": [ { "city": "Köln", "events": 38 } ],
  "last_run": null
}
```

`weekly` holds the last 8 weeks, oldest first, counting new rows by their creation week. `last_run` is a run object as in `GET /api/runs`, or null.

`GET /api/events?from=&to=&city=&type=&fit=kept|dropped|all&q=`

Defaults: `from` today, `to` from plus `collect_ahead_days`, `fit` kept. Each event is a public event plus these fields:

```json
{
  "fit": "kept", "fit_reason": "keep word: pitch",
  "people": [{ "id": 7, "name": "Lea Beispiel", "role": "pitch", "affiliation": "Beispiel GmbH" }],
  "sources": [{ "id": 3, "name": "Startplatz events", "url": "https://www.startplatz.de/events" }],
  "first_seen": "2026-09-25T03:12:00Z"
}
```

In private responses `people` always holds everyone named, each with their `id`.

`PATCH /api/events/{id}` with `{"fit": "kept" | "dropped", "fit_reason": "optional"}` answers the updated event.

`GET /api/people?q=&sort=next|new|name&filter=all|upcoming|profile|founder`

The answer is `{"people": [...], "counts": {"all": 57, "founder": 3, "upcoming": 40, "profile": 12}}`. `counts` says how many people each filter shows for the same search, so an empty filter never hides the others.

`sort=next` (the default) orders by the next upcoming appearance, people without one last. `new` orders by first seen, newest first. `filter=upcoming` keeps people with an upcoming appearance, `profile` keeps people with at least one profile that is not rejected, and `founder` keeps people whose `fit` is `founder`: an event page says they founded or run something, or their title says so.

```json
{
  "people": [
    {
      "id": 7, "name": "Lea Beispiel", "known_as": "", "headline": "Founder, Beispiel GmbH", "city": "Köln",
      "fit": "founder", "first_seen": "2026-09-25T03:12:00Z", "appearances": 2,
      "next_appearance": { "event_id": 12, "title": "Rheinland Pitch #129", "starts_at": "2026-09-28T16:00:00Z", "city": "Köln", "role": "pitch" },
      "profiles": [{ "id": 4, "platform": "linkedin", "url": "https://www.linkedin.com/in/example", "review": "open" }]
    }
  ]
}
```

`GET /api/people/{id}` answers one person with the list fields plus:

```json
{
  "notes": "",
  "fit_evidence": "hat die Backstube Muster gegründet",
  "appearances": [{ "role": "pitch", "evidence": "...", "event": { "id": 12, "title": "...", "starts_at": "...", "city": "Köln", "venue": "Startplatz", "url": "..." } }],
  "affiliations": [{ "organisation": "Beispiel GmbH", "role": "founder", "current": true }],
  "sightings": [{ "source_id": 3, "source": "Startplatz events", "checked_at": "..." }]
}
```

`evidence` is the passage from the event page that puts the person on stage, when the local model found them. It is empty when the rules found them.

`PATCH /api/people/{id}` with `{"notes": "..."}` answers the updated person.

`PATCH /api/profiles/{id}` with `{"review": "open" | "confirmed" | "rejected"}` answers 204.

`GET /api/sources?status=&q=`

```json
{
  "sources": [
    {
      "id": 3, "name": "Startplatz events", "kind": "listing", "url": "https://www.startplatz.de/events", "query": null,
      "category": "Venue and organiser", "city": "Köln", "status": "active", "fetch_mode": "auto", "notes": "",
      "last_checked_at": "...", "next_check_at": "...", "checks": 4, "empty_checks_in_row": 0, "points": 0,
      "last_check": { "events_found": 7, "http_status": 200, "mode": "http", "error": "" },
      "health": "ok", "health_note": "",
      "discovered_from": "Imported from the brief"
    }
  ]
}
```

`health` is `ok`, `warning`, `error` or `never` (not checked yet). `last_check` is null before the first check.

`POST /api/sources` with `{"url": "...", "name": "optional"}` adds a candidate source and answers it with 201. A link to one event on Meetup, Luma or Eventbrite adds the calendar it belongs to. LinkedIn, Instagram and Facebook answer 400, an address that is already a source 409.

`PATCH /api/sources/{id}` with any of `{"status", "fetch_mode", "notes", "name"}` answers the updated source.

`POST /api/sources/{id}/check` queues a check now in its own run of kind `check` and answers 202 with `{"run_id": 7}`.

`GET /api/runs` answers the last 50 runs:

```json
{
  "runs": [
    {
      "id": 5, "kind": "nightly", "started_at": "...", "finished_at": "...",
      "sources_checked": 42, "events_found": 180, "events_new": 23, "pages_read": 30, "people_new": 41, "errors": 3
    }
  ]
}
```

`pages_read` counts the event pages the local model read in the run, and `people_new` includes the people it found there. `kind` is `nightly`, `manual` for a run started by hand, or `check` for Check now. `finished_at` is null while the run is going. A run that `serve` works on records no end of its own, so it counts as finished once none of its checks wait any more.

`POST /api/runs` starts a run by hand: every due source, as the nightly run would check them. The worker in `serve` works on it. It answers 202 with `{"run_id": 8, "started": true}`, or with the run still going and `"started": false`.

While a run is going, it carries `progress`, and is null otherwise:

```json
{ "checks": 12, "checks_done": 7, "reads": 6, "reads_done": 1,
  "now": [{ "kind": "read", "label": "Pitch Abend Köln", "since": "..." }],
  "seconds_left": 140 }
```

`now` lists what runs at this moment. `seconds_left` is measured from how long the last 200 checks and 100 reads took, checks four at a time and reads one at a time. It is null until there is anything to measure against.

`GET /api/runs/current` answers `{"run": {...}}` with the run that is going, or `{"run": null}`.

When the app starts, runs it was working on when it stopped end, and their work does not come back by itself. A run drops checks and reads left over from earlier runs. An event page whose read failed three times is not read again.

`POST /api/runs/{id}/stop` stops a run by hand and answers 204. Its queued checks and reads are dropped, and what is running finishes. A run that already ended answers 409.

`GET /api/runs/{id}` answers `{"run": {...}, "checks": [...]}`, where each check is:

```json
{
  "id": 91, "source": { "id": 3, "name": "Startplatz events", "url": "..." },
  "mode": "http", "http_status": 200, "events_found": 7, "events_kept": 5, "people_found": 9,
  "error": "", "duration_ms": 1830, "checked_at": "...", "fetch_id": 311
}
```

`GET /api/fetches/{id}/text`, `/html` and `/screenshot` answer the stored visible text, raw HTML and PNG screenshot of one fetch, or 404 once they are older than 30 days.


`GET /api/settings` answers `{"settings": [{"key", "value", "description", "updated_at"}]}`.

`PATCH /api/settings/{key}` with `{"value": <any JSON>}` answers the updated setting.
