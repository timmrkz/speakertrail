# HTTP API

The interface talks to the app through this JSON API. It runs inside the
app, behind its window, and nothing outside the app can reach it, so there
is no login. `internal/api` answers it.

All times are RFC 3339 strings in UTC. Errors are always
`{"error": "message"}` with a fitting status code. POST, PATCH and PUT take
only JSON with `Content-Type: application/json`, anything else gets 415.

The shapes are the ones the interface knew from the server that ran in
Docker, so some fields say little in the app: a person has no appearances,
a search has no URL. They are there so the interface needs no second set of
types.

## People

`GET /api/people?q=lea&sort=young&filter=all`

`sort` is `young` (the default), `new`, `fit` or `name`. `filter` is `all`,
`young`, `kept` or `skipped`. `q` looks in the name, the headline and the
city. `counts` says how many people each filter would show.

```json
{
  "people": [
    {
      "id": 7,
      "name": "Lea Beispiel",
      "headline": "Gründerin, Studio Beispiel",
      "city": "Köln",
      "first_seen": "2026-11-10T18:00:00Z",
      "fit_score": 3,
      "decision": "",
      "signals": [{ "key": "founder", "label": "Founded something", "for": true, "passage": "Gründerin", "found_in": "LinkedIn" }],
      "profiles": [{ "id": 7, "platform": "linkedin", "url": "https://www.linkedin.com/in/lea-beispiel", "review": "" }],
      "linkedin": { "born_about": 2001, "age_reason": "Bachelor finished 2022", "roles": "Gründerin, Studio Beispiel", "last_active": "2026-10-10T00:00:00Z" }
    }
  ],
  "counts": { "all": 1, "young": 1, "kept": 0, "skipped": 0 }
}
```

`decision` is `kept`, `skipped` or empty. `linkedin` is null once the
profile is rejected.

`GET /api/people/{id}` gives one person, with `notes`, `fit_evidence` and
`sightings`: each search that found the person, newest first.

`PATCH /api/people/{id}` with `{"decision": "kept"}`, `{"decision": ""}`
or `{"notes": "..."}` answers the person as `GET` does. A decision other
than `kept`, `skipped` or empty gets 400.

`PATCH /api/profiles/{id}` with `{"review": "confirmed"}`, `"rejected"` or
`"open"`. A person has one profile, so its id is the person's.

## Searches

The interface calls them sources, so the paths say `sources`. Every one is
a profile search.

`GET /api/sources?kind=profile_search&q=köln` lists them, with how many
people each found and how many of those are kept.

`POST /api/sources` with `{"query": "Yogalehrerin Bonn"}` adds a search and
answers 201 with it. The same search again, in any case, gets 409.

Only profile searches can be added, another `kind` gets 400.

`PATCH /api/sources/{id}` with `{"status": "retired"}` pauses a search, any
other status resumes it as a candidate. `{"notes": "..."}` keeps a note.

`POST /api/sources/{id}/check` asks this one search now, as a run of its
own, and answers 202. While a run goes it gets 409.

## Runs

`POST /api/runs` starts a run of the due searches and answers
`{"run_id": 3, "started": true}`. When a run already goes, `started` is
false and `run_id` is that run. Without an Exa key, with the month's
budget spent, or with no search due it gets 409 and says which.

`GET /api/runs/current` answers `{"run": null}` or the going run, with
`progress`: how many searches it asks, how many are done, which ones are
being asked now and since when, and `seconds_left`, measured from the last
200 searches asked. `seconds_left` is null until there is something to
measure.

`POST /api/runs/{id}/stop` stops a going run and answers 204, or 409 when
that run is not going.

`GET /api/runs` lists the last 50 runs, newest first. `GET /api/runs/{id}`
gives one, with its `checks` and its `failures`.

A run's `state` is `going`, `finished`, `stopped`, or `interrupted` when
the app was closed while it went.

## Settings and the key

`GET /api/settings` lists every setting with its value and what it
does.
`PATCH /api/settings/{key}` with `{"value": 5}` changes one. A value of the
wrong kind gets 400, an unknown key 404.

`GET /api/keys` says whether the Exa key is saved and how many searches
this month used, never the key itself:

```json
{ "keys": [{ "provider": "exa", "used": 12, "budget": 1000, "set": true }] }
```

`PUT /api/keys/exa` with `{"key": "..."}` saves the key in the keychain and
answers 204. An empty key removes it.

`GET /api/me` always answers `{"logged_in": true}`.
