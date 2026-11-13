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
`young` or `connected`. `q` looks in the name, the headline and the city.
`counts` says how many people each filter would show.

People whom one of Tim's exclusions leaves out are not in the list or the
counts, unless Tim connected with them, see [Exclusions](#exclusions).

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
      "connected": false,
      "signals": [{ "key": "founder", "label": "Founded something", "for": true, "passage": "Gründerin", "found_in": "LinkedIn" }],
      "profiles": [{ "id": 7, "platform": "linkedin", "url": "https://www.linkedin.com/in/lea-beispiel", "review": "" }],
      "linkedin": { "born_about": 2001, "age_reason": "Bachelor finished 2022", "roles": "Gründerin, Studio Beispiel", "last_active": "2026-10-10T00:00:00Z" }
    }
  ],
  "counts": { "all": 1, "young": 1, "connected": 0 }
}
```

`connected` says whether Tim connected with the person on LinkedIn.
`linkedin` is null once the profile is rejected.

`GET /api/people/{id}` gives one person, with `notes`, `fit_evidence` and
`sightings`: each search that found the person, newest first.

`PATCH /api/people/{id}` with `{"connected": true}`, `{"connected": false}`
or `{"notes": "..."}` answers the person as `GET` does.

`PATCH /api/profiles/{id}` with `{"review": "confirmed"}`, `"rejected"` or
`"open"`. A person has one profile, so its id is the person's.

## Searches

The interface calls them sources, so the paths say `sources`. Every one is
a profile search.

`GET /api/sources?kind=profile_search&q=köln` lists them, with how many
people each found, `people`, and how many of those Tim connected with,
`people_connected`.

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
does, except where LinkedIn opens, which has its own endpoints below.
`PATCH /api/settings/{key}` with `{"value": 5}` changes one. A value of the
wrong kind gets 400, an unknown key 404.

`GET /api/keys` says whether the Exa key is saved and how many searches
this month used, never the key itself:

```json
{ "keys": [{ "provider": "exa", "used": 12, "budget": 1000, "set": true }] }
```

`PUT /api/keys/exa` with `{"key": "..."}` saves the key in the keychain and
answers 204. An empty key removes it.

## Where LinkedIn opens

The LinkedIn button opens a profile in the browser profile that is logged
in to LinkedIn, with `open -n -a <browser> --args
--profile-directory=<folder> <link>`. The app never asks LinkedIn for
anything itself. See `internal/browser`.

`GET /api/linkedin` answers the choice and what this Mac has, read from
each Chromium browser's `Local State` every time:

```json
{
  "app": "Google Chrome", "folder": "Profile 1", "known": true,
  "browsers": [{ "app": "Google Chrome", "profiles": [{ "folder": "Default", "name": "Privat" }, { "folder": "Profile 1", "name": "Arbeit" }] }]
}
```

`known` is false when nothing is chosen or the chosen profile is gone. One
browser with one profile counts as chosen.

`PUT /api/linkedin` with `{"app": "Google Chrome", "folder": "Profile 1"}`
chooses. A browser or profile that is not on this Mac gets 400.

`POST /api/open` with `{"url": "https://..."}` opens the link there and
answers 204. Only https links open, anything else gets 400. With nothing
chosen, or a browser that is gone, it answers 409 with what to do.

`GET /api/me` always answers `{"logged_in": true}`.

## Updates

`/api/updates` is answered by the app itself, not by `internal/api`. See
[UPDATES.md](UPDATES.md).

## Exclusions

An exclusion is a word or phrase, like "Online Marketing". A profile
search leaves out anyone whose headline or current roles contain one, and
`GET /api/people` hides those found before, unless Tim connected with
them. A phrase matches at the start of a word, in any case, whatever
stands between its words, and also written as one word: "Online
Marketing" matches "Online-Marketing-Expertin" and "#onlinemarketing", but
"Coach" does not match "Lifecoach".

`GET /api/exclusions` answers `[{"id": 1, "phrase": "Online Marketing",
"people": 3}]`, in the order of the phrases. `people` is how many people
it hides now. Someone two exclusions match counts for the first.

`POST /api/exclusions` with `{"phrase": "Personal Branding"}` adds one and
answers it with 201. The phrase keeps its words, with single spaces
between them, at most 60 characters. The same phrase twice answers 409,
one without a word 400.

`DELETE /api/exclusions/{id}` removes one and answers 204. The people it
hid come back.
