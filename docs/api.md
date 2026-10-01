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
  "last_run": null,
  "searches": [
    { "provider": "exa", "used": 42, "budget": 1000, "set": true },
    { "provider": "tavily", "used": 0, "budget": 1000, "set": false },
    { "provider": "brave", "used": 0, "budget": 1000, "set": false }
  ]
}
```

`searches` holds each search provider the engine knows, with how many searches it made this month, counted from the first of the month in Berlin, and its budget from Settings, `exa_monthly_searches`, `tavily_monthly_searches` and `brave_monthly_searches`. `set` is false when the provider has no API key, and then it is never called.

`fit` says what the keeps and skips teach the fit rubric: `signals` lists every signal of the rubric, in its order, with how often a person who had it was `kept` or `skipped`, and `top` how the top 20 by fit were decided, `{"size": 20, "kept": 6, "skipped": 3, "open": 11}`.

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
  "affiliations": [{ "organisation": "Beispiel GmbH", "role": "founder", "current": true,
    "website": "https://beispiel.example/", "imprint_url": "https://beispiel.example/impressum" }],
  "sightings": [{ "source_id": 3, "source": "Startplatz events", "checked_at": "..." }]
}
```

`website` and `imprint_url` are the business's, empty when no lookup found them. They are how Tim reaches someone found through their website: the imprint gives an email address, by law. The app stores the links, never the address or a phone number. A person who runs a business alone also gets every profile its website links, like the studio's Instagram, to confirm or reject. With several people in the imprint, only profiles whose address carries the person's name are theirs.

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
      "last_check": { "events_found": 7, "startups_found": 0, "http_status": 200, "mode": "http", "error": "" },
      "health": "ok", "health_note": "",
      "discovered_from": "Imported from the brief"
    }
  ]
}
```

`health` is `ok`, `warning`, `error` or `never` (not checked yet). `last_check` is null before the first check. A source of kind `portfolio` lists startups, not events, and one of kind `directory` lists businesses run by people, like gyms or coaches. Their health counts `startups_found`, for a directory only those in NRW.

A check of a portfolio stores each startup its page links to, with its website, or with the portfolio's own page about it, and follows the list to its next pages, six pages at most. A lookup then finds the website on that page when needed, loads it, finds its imprint and stores the managing directors it names as founders, when the imprint reads like a young company: a GmbH, UG or sole trader with at most four managing directors. A startup whose site does not answer is looked up again by a later run, three times at most. Only names are taken from an imprint.

A directory works the same way, with two differences. Directories are national, so only NRW counts: an entry whose postcode on the list lies outside NRW is not kept, and a lookup whose imprint gives no postcode in NRW takes nobody, noted as "outside NRW (80331 München)". And an owner, "Inhaber", is the best case, so businesses from directories are looked up before startups from portfolios. A first and last name that two imprints in the same city give is one person, who runs both businesses.

A search, kind `search_query`, has a `query` and no `url`. Its check asks a search provider, keeps one website per host, leaves out LinkedIn, Instagram, platforms and list sites like Yelp or Eventbrite, and stores each website as a business, like a directory's entry. Only NRW counts, by the imprint's postcode. Each search goes to the provider with the largest share of its budget left, and to the next one when it fails. Once every budget is spent, or when no provider has a key, searches wait. A run takes `searches_per_run` due searches, and a search runs again after `search_check_days`. Its health counts `startups_found`, the websites it kept.

`POST /api/sources` with `{"url": "...", "name": "optional", "lists": "events"}` adds a candidate source and answers it with 201. `lists` is `events`, `startups` for a portfolio or `businesses` for a directory. `"portfolio": true` still means startups. A link to one event on Meetup, Luma or Eventbrite adds the calendar it belongs to. LinkedIn, Instagram and Facebook answer 400, an address that is already a source 409.

`POST /api/sources` with `{"query": "BJJ Gym Köln"}` adds a search instead, a candidate source of kind `search_query`, and answers it with 201. Its name is the query unless `name` is given. A search that is already a source answers 409, whatever its case.

`PATCH /api/sources/{id}` with any of `{"status", "fetch_mode", "notes", "name", "lists"}` answers the updated source. `lists` switches what the page is read as, and the source is checked in the next run. A search has no page, so `lists` on a search answers 400.

`POST /api/sources/{id}/check` queues a check now in its own run of kind `check` and answers 202 with `{"run_id": 7}`.

A person in `GET /api/people` carries the fit rubric's verdict, see [search-strategy.md](search-strategy.md). `signals` lists what speaks for and against a fit, those for first, each with the passage that shows it and where the passage comes from: `title`, `event page`, `event`, `imprint`, `lookup`, `portfolio`, `about page` or `model`. A lookup reads the website's about page, "Über mich" or "Über uns", for what it says about the people the imprint names. A passage with an email address or a phone number is never kept. `fit_score` is the number of signals for, less those against. A claim without a passage does not count.

```json
{ "fit_score": 1, "signals": [
  { "key": "works_with_people", "label": "Works with people", "for": true, "passage": "Yoga-Lehrerin, Studio Beispiel", "found_in": "title" },
  { "key": "owner_operator", "label": "Runs it themselves", "for": true, "passage": "Inhaberin des Studios", "found_in": "model" },
  { "key": "backed", "label": "Backed by a startup programme", "for": false, "passage": "In the portfolio of Beispiel Hub", "found_in": "portfolio" } ] }
```

`decision` is `kept`, `skipped`, or empty when Tim has not decided. `PATCH /api/people/{id}` with `{"decision": "kept"}` keeps a person, `"skipped"` skips them and `""` undoes either. Each decision is recorded with the signals the person had then, and still counts after a skipped person is deleted. Skipped people come last in `fit` order and are not among the good fits.

`sort` is `new` for the newest first, `next` for the next appearance, `name`, or `fit` for the best fits first. `filter` is `all`, `fits` for a score above 0, `upcoming`, `profile` or `founder`, and `counts` has each.

A person also carries `activity`, what the last lookup saw of the startup they founded, or null: `{"state": "quiet", "since": "2024-05-02T00:00:00Z", "note": "the website changed May 2024, by its sitemap", "company": "Beispiel GmbH"}`. `state` is `active`, `unknown`, `quiet`, `dissolved` or `gone`. Sorted by next appearance, founders of active startups come before the others.

`GET /api/runs` answers the last 50 runs:

```json
{
  "runs": [
    {
      "id": 5, "kind": "nightly", "state": "finished", "started_at": "...", "finished_at": "...",
      "sources_checked": 42, "events_found": 180, "events_new": 23, "pages_read": 30, "reads_failed": 2,
      "startups_looked_up": 10, "lookups_failed": 1, "people_new": 41, "fits_new": 6, "errors": 3
    }
  ]
}
```

`pages_read` counts the event pages the local model read in the run, `startups_looked_up` the startups whose imprint it looked up, and `people_new` includes the people both found. `fits_new` counts the new people who fit, like founders. `errors` counts the checks that failed, `reads_failed` and `lookups_failed` the reads and lookups.

`state` is the run's true state: `going`, `finished`, `stopped` by hand, or `interrupted` when the app stopped under it and the run ended as the app started again. A nightly run cut short, like `make crawl` stopped with Ctrl-C, is `stopped`. `kind` is `nightly`, `manual` for a run started by hand, or `check` for Check now. `finished_at` is null while the run is going. A run that `serve` works on records no end of its own, so it counts as finished once none of its checks wait any more.

`POST /api/runs` starts a run by hand: every due source, as the nightly run would check them. The worker in `serve` works on it. It answers 202 with `{"run_id": 8, "started": true}`, or with the run still going and `"started": false`.

While a run is going, it carries `progress`, and is null otherwise:

```json
{ "checks": 12, "checks_done": 7, "reads": 6, "reads_done": 1, "reads_expected": 8,
  "lookups": 10, "lookups_done": 4, "lookups_expected": 10,
  "now": [{ "kind": "read", "label": "Pitch Abend Köln", "since": "..." }],
  "seconds_left": 140 }
```

`now` lists what runs at this moment, a `check`, a `read` or a `lookup`. `reads_expected` and `lookups_expected` count what checks still to come will bring. `seconds_left` is measured from how long the last 200 checks, 100 reads and 100 lookups took, checks and lookups four at a time and reads one at a time. It is null until there is anything to measure against.

`GET /api/runs/current` answers `{"run": {...}}` with the run that is going, or `{"run": null}`.

When the app starts, runs it was working on when it stopped end, and their work does not come back by itself. A run drops checks, reads and lookups left over from earlier runs. An event page whose read failed three times is not read again.

`POST /api/runs/{id}/stop` stops a run by hand and answers 204. Its queued checks, reads and lookups are dropped, and what is running finishes but queues nothing more. A run that already ended answers 409.

`GET /api/runs/{id}` answers `{"run": {...}, "checks": [...], "failures": [...]}`. Each failure says in plain words what went wrong, one per source for checks and one per reason for reads and lookups:

```json
{ "what": "check", "source": { "id": 3, "name": "Startplatz events", "url": "...", "status": "active" },
  "reason": "The site did not answer in time", "action": "check", "count": 1,
  "detail": "get https://...: context deadline exceeded" }
```

`what` is `check`, `read` or `lookup`, and `source` is null for reads and lookups. `action` is the one thing that fixes it: `retire` the source, `check` it again, or empty when the engine already made the source manual or retired it, or when no source is to blame, like a language model that did not answer. A source that failed its last three checks is worth retiring. `detail` is the error as recorded.

Each check is:

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
