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
      "linkedin": { "url": "https://www.linkedin.com/in/lea-beispiel", "born_about": 2001, "age_reason": "Bachelor finished 2022", "roles": "Gründerin, Studio Beispiel", "last_active": "2026-10-10T00:00:00Z" }
    }
  ],
  "counts": { "all": 1, "young": 1, "connected": 0 }
}
```

`connected` says whether Tim connected with the person on LinkedIn.
`linkedin` is null once the profile is rejected.

`GET /api/people/{id}` gives one person, with `notes`, `fit_evidence` and
`sightings`: each search that found the person, newest first. `draft` is
the first message to them, empty before the first draft, and
`drafted_at` when the language model wrote it. `profile_read_at` is when
Exa's index gave the profile text a draft is written from, null while the
app has none and a draft reads it first. See [Drafts](#drafts).

`PATCH /api/people/{id}` with `{"connected": true}`, `{"connected": false}`,
`{"notes": "..."}` or `{"draft": "..."}` answers the person as `GET`
does.

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
does. Where LinkedIn opens is not one of them, it has its own endpoints
below. `PATCH /api/settings/{key}` with `{"value": 5}` changes one. A
value of the wrong kind gets 400, an unknown key 404.

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

`POST /api/people/{id}/linkedin` with `{}` opens that person's LinkedIn
profile there and answers 204. The interface names the person and the
app takes the link from its file, so it opens nothing else. A person
without a profile, or whose profile is rejected, gets 404. With nothing
chosen, or a browser that is gone, it answers 409 with what to do and
`"choose": true`, and the interface leads to Settings.

`GET /api/me` always answers `{"logged_in": true}`.

## Updates

`/api/updates` is answered by the app itself, not by `internal/api`. See
[UPDATES.md](UPDATES.md).

## The app's log

`/api/log` is answered by the app itself, not by `internal/api`. See
[LOGGING.md](LOGGING.md) and `cmd/speakertrail-app/applog_api.go`.

| Request | What it does |
| --- | --- |
| `POST /api/log/said` | the window's lines, `[{"at": 1763215200000, "level": "warn", "act": "a3", "toast": false, "text": "…"}]`, at most 400 written from one request, each cut at 2000 characters, the body at most 2 MB |
| `POST /api/log/acted` | the person's act, `{"id": "a3", "at": 1763215200000, "what": "pressed button.btn \"Run\""}` |
| `GET /api/log` | the log as Settings shows it, `{"path": "…/app.log", "files": 2, "bytes": 2430000, "lines": 9812, "toasts": [{"at": 1763215200000, "text": "…"}]}` |
| `POST /api/log/watch` | `{"on": true}` while Settings is open, so the log is sent as the Wails event `applog` as lines are written, at most once a second |
| `POST /api/log/show` | shows the log in Finder, 409 while there is none |
| `DELETE /api/log` | removes every file of the log, and nothing else in its folder, and answers the log as it is then |

The two the window says the log with are sent as they are, never timed
or said themselves. Every other request the window makes is timed, see
What the window says in LOGGING.md.

## The title bar

`GET /api/chrome` is answered by the app itself, not by `internal/api`.
It says how tall the title bar macOS laid out is and where its three
buttons are, in whole CSS pixels, `{"bar": 30, "left": 12, "right": 72}`.
All zeros mean the system draws its own title bar. The page asks once
when it starts and hears every change after on the Wails event called
`chrome`. `GET /api/chrome?window=report` answers the same for the
report window, whose changes come as `chrome-report`. See
`cmd/speakertrail-app/chrome.go`.

## Quitting

Cmd+Q dims the app at once and says what happens next, the way
FrameFairy does. The first press only asks, Press ⌘Q again to quit, for
three seconds, the way Chrome does, on every page and whether a run goes
or not. A click or Escape takes the question away, and the next press
asks again. The second press says Quitting while the app stops a run that
goes and puts a ready build in place, and then it goes. Closing the
window quits without asking, and so does **Relaunch** on the Updates
page, which was asked for with a click. So does a logout, restart or
shutdown of the Mac, which a question would cancel. The stopping happens
away from the main thread, see `cmd/speakertrail-app/quit.go`.

The Go side tells the page on the Wails event called `quit`,
`{"what": "ask", "ms": 3000}` for the first press, with how long the
question stays, and `{"what": "going"}` once the app is on its way out.
The page answers over HTTP, answered by the app itself, not by
`internal/api`:

`POST /api/quit/stay` is the question taken away, with Escape or a click,
so the next Cmd+Q asks again rather than quitting. It answers 204.

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

## Speech models

`/api/speech` is answered by the app itself, not by `internal/api`. It is
what Settings shows under Speech models, see
`cmd/speakertrail-app/speech_api.go`. The models live in
`~/Library/Application Support/Speaker Trail/models`.

| Request | What it does |
| --- | --- |
| `GET /api/speech` | every model, whether it is on this Mac, the folder written with `~`, and the download under way, `{"models": [{"name": "…", "installed": false, …}], "folder": "~/Library/…/models", "job": {"id": "speech-1", "state": "running", "progress": {…}}}` |
| `POST /api/speech/install` | `{"name": "…"}` fetches that model and answers the download at once |
| `POST /api/speech/cancel` | `{"id": "speech-1"}` stops that download or its unpacking. What arrived stays and the next Download goes on from there, and an archive that arrived whole is unpacked again without being fetched again |
| `DELETE /api/speech/models/{name}` | takes that model off the Mac, after Settings asked, and lets go of it if Dictate had it loaded |

`progress` says what the download does now in `text`, `fetching …` or
`unpacking`, the share done from 0 to 1 in `fraction` and the seconds left
in `remaining`, each -1 where it cannot be said yet. While it unpacks, the
share is of what the model takes once unpacked, and the seconds left are
at the pace it has kept so far.

Where a download stands is sent as the Wails event `speech`, in the same
shape as `GET /api/speech`.

## Language models

`/api/language` is answered by the app itself, the same as
[Speech models](#speech-models), see
`cmd/speakertrail-app/languagemodels.go`. It is what Settings shows under
Language model. The models live in the same folder.

| Request | What it does |
| --- | --- |
| `GET /api/language` | every model, largest first, whether it is on this Mac, the folder written with `~`, whether the app carries llama-server to run one, and the download under way, `{"models": [{"name": "…", "title": "Gemma 4 26B A4B", "download": 14439363584, "needs": 16624595968, "fit": "fits", "recommended": true, "installed": false, "inUse": false, …}], "folder": "~/Library/…/models", "server": true, "job": {…}}` |
| `POST /api/language/install` | `{"name": "…"}` fetches that model and answers the download at once |
| `POST /api/language/cancel` | `{"id": "language-1"}` stops that download. What arrived stays and the next Download goes on from there |
| `DELETE /api/language/models/{name}` | takes that model off the Mac, after Settings asked. Not while a draft writes with it |

A model FrameFairy has whole in `~/.framefairy/models`, by its size and
checksum, is linked from there rather than downloaded, so the two apps
share it on disk. While it is checked, `progress.text` is `copying`.

`needs` is the memory the model takes while it writes, in bytes, and
`fit` what this Mac can do with it: `fits`, `tight`, `too big` or
`unknown`. `recommended` marks the largest model this Mac holds
comfortably, the one Settings offers. `inUse` marks the one that drafts,
the largest on this Mac. `job` is in the shape of a speech model's
download.

Where a download stands is sent as the Wails event `language`, in the
same shape as `GET /api/language`.

## Drafts

A draft is a first message to a person, written on the Mac by the
language model from their profile and the prompt in Settings,
`first_message_prompt`. See `internal/draft`. Drafts are written one at a
time, in the order they were asked for.

| Request | What it does |
| --- | --- |
| `GET /api/drafts` | the latest draft of each person since the app started, oldest first |
| `POST /api/people/{id}/draft` | `{}` starts a draft for them and answers it at once. Asked again while theirs is under way, it answers that one |
| `POST /api/people/{id}/draft/cancel` | `{}` stops theirs, 204. 409 when none is under way |

A draft is

```json
{ "id": "draft-3", "person_id": 7, "state": "writing", "model": "Gemma 4 26B A4B", "text": "Hallo Lea, danke", "started_at": "2026-11-19T14:00:00Z", "fraction": 0.6, "seconds_left": 4 }
```

`state` is `queued`, `reading` (their profile from Exa, when the app has
no text of it yet), `loading` (the model, when it is not loaded), `writing`,
`done`, `failed` or `cancelled`. `text` is what the model wrote so far, and
the whole message once it is done. It is untrusted and shown as text.
`fraction` and `seconds_left` come from how long earlier drafts took, -1
and null before there was one. `error` says why a draft failed, in a
sentence that can go on screen.

A draft that is done is kept on the person as `draft`. Reading a profile
counts against Exa's monthly budget like a search. Each change is sent as
the Wails event `draft`, one draft at a time, and once a second while one
runs. Where the app runs no language model, starting one answers 409.

## Report a Problem

`/api/report` is answered by the app itself, not by `internal/api`. It
keeps the draft of a report in `~/Library/Application Support/Speaker
Trail/report`, so a draft outlives the window and the app. See
`cmd/speakertrail-app/report_api.go`, `bugs.go` and `report.go`. Errors
answer 409 with a sentence that can go in a toast.

| Request | What it does |
| --- | --- |
| `GET /api/report` | the draft, `{"text": "…", "pictures": [{"id": "p1", "name": "Screenshot….png", "width": 1440, "height": 900}]}`. Pictures removed a moment ago are left out |
| `GET /api/report/window` | `{"open": true}` while the report window is open |
| `POST /api/report/open` | `{"how": "bugs"}` opens the report window, or brings it to the front |
| `POST /api/report/about` | `{"text": "…"}` opens it with the words of a toast after what was written |
| `PUT /api/report/text` | `{"text": "…"}` keeps what was written |
| `GET /api/report/picture?id=p1` | the picture itself, as PNG |
| `POST /api/report/pick` | asks for pictures in a Finder dialog, PNG or JPEG |
| `POST /api/report/pasted` | `{"name": "…", "data": "<base64>"}` adds a pasted picture |
| `POST /api/report/remove` | `{"id": "p1"}` takes a picture out, and keeps it aside for Put it back |
| `POST /api/report/put-back` | `{"id": "p1"}` puts it back |
| `POST /api/report/clear` | empties the draft, and keeps it aside for Undo |
| `POST /api/report/send` | `{"what": "…"}` makes the report, a zip on the Desktop, shows it in Finder, closes the window and answers `{"path": "~/Desktop/Speaker Trail report 2026-11-16 10.15.zip"}` |

At most 10 pictures, each at most 10 MB. The draft as it changes is sent
as the Wails event `report-draft`, and whether the window is open as
`report-window`. What goes into the zip is in
[LOGGING.md](LOGGING.md), Report a Problem.

## Dictate

`/api/listen` is answered by the app itself, not by `internal/api`. Every
request is a POST. See `cmd/speakertrail-app/listen.go`.

| Request | What it does |
| --- | --- |
| `POST /api/listen` | opens the microphone and answers `{"recording": 3}`. Without a speech model 409 says to download it in Settings |
| `POST /api/listen/stop` | closes it and answers what was said, `{"recording": 3, "said": ["…"], "hearing": ""}` |
| `POST /api/listen/settle` | `{"recording": 3, "pieces": 2, "text": "…"}`, the window took those pieces into its field |
| `POST /api/listen/break` | `{"recording": 3, "pieces": 2}`, the window took them in and moved on, so the next piece starts afresh |
| `POST /api/listen/cancel` | closes it and drops what was heard, 204 |

While the microphone is open, how loud it is goes out as the Wails event
`loudness`, `{"recording": 3, "level": 0.4}`, and each piece the speech
model wrote down as `dictation`.

## Undo

Edit, Undo and Redo in the app's menu do not reach the page as keys. The
app sends the window in front the Wails event `undo`, `"undo"` or
`"redo"`, and the page decides: Report a Problem undoes its own steps,
and a text field anywhere undoes what was typed. See
`cmd/speakertrail-app/menu.go`.
