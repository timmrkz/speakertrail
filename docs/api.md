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

`GET /api/people?q=lea&sort=fit&filter=all`

`sort` is `fit` (the default, the best fit first), `young`, `new` or `name`. `filter` is `all`,
`new`, `connected` or `rejected`. `q` looks in the name, the headline and the city.
`counts` says how many people each filter would show.

Someone Tim connected with is only under `connected`, and someone whose
profile he rejected only under `rejected`. `all` holds the
people he has neither connected with nor rejected, so it shrinks as he
goes through them. Someone connected stays under `connected` even when
their profile is rejected. `new` holds whoever came in during the last 7
days, found by a search or added from a link, connected or not, never
someone whose profile he rejected, and always the newest first.

When `q` holds links, like LinkedIn profile links pasted into the search,
it is no search for words. `links` then says what adding them would do:
`new` profiles nobody here has, `here` the ids of the people the links
lead to, and `other` links that are no person's profile, like a company
page. `act` is what Run turns into: `add` while any profile is new,
`open` for one link of someone here and nothing else, and `none` when
the links add nobody. `people` are those here, under any filter. Without
links, `links` is null.

```json
{ "people": [], "counts": { "all": 4, "new": 2, "connected": 1, "rejected": 0 }, "links": { "new": 2, "here": [], "other": 1, "act": "add" } }
```

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
      "fit_score": 64,
      "connected": false,
      "signals": [{ "key": "founder", "label": "Founded something", "for": true, "passage": "Gründerin", "found_in": "LinkedIn" }],
      "profiles": [{ "id": 7, "platform": "linkedin", "url": "https://www.linkedin.com/in/lea-beispiel", "review": "" }],
      "linkedin": { "url": "https://www.linkedin.com/in/lea-beispiel", "born_about": 2001, "age_reason": "Bachelor finished 2022", "roles": "Gründerin, Studio Beispiel", "last_active": "2026-10-10T00:00:00Z" }
    }
  ],
  "counts": { "all": 1, "new": 1, "connected": 0, "rejected": 0 }
}
```

`connected` says whether Tim connected with the person on LinkedIn.
`fit_score` is their fit from 0 to 100, worked out as of now and never
kept, see [fit-score.md](fit-score.md).
`linkedin` is null once the profile is rejected.

`read` says where the profile of someone added from a link stands:
`reading` now, with `read_since` when it began and `read_ms` about how
long a read takes, measured from the last reads and null until there is
one, `waiting` its turn in the run, `next` for the next run, which reads
it, `not_indexed` when Exa's index does not hold it yet, neither under
the link nor by the name in the link, `failed` with
`read_error` when the last read failed, or empty once read. A run reads
the profiles before it asks its searches.

`POST /api/people` with `{"text": "…"}` adds the people behind the
LinkedIn profile links in the text, as connected and named by the handle
in the link, and has a run read their profiles: the run going, or a run
of its own that reads only those and asks no search. Words and other
links in the text are passed over. Someone here already is only marked
connected. It answers `{"run": 12, "new": [31, 32], "here": [7],
"connected": [7], "other": 1}`: `connected` are those here whom it marked
connected, and `open` is the one person here when the links lead to that
person alone. `run` is 0 when there is no profile to read: without the
Exa key or budget the people are added all the same, and `wait` says why
their profiles wait for the next run. A profile the going run read
already, or failed to, waits for the next run too, so no run reads one
twice. Someone whose profile Exa did not have, or could not give, is
asked for again when added again. 400 when the text holds no profile
link. Each read is one call to Exa's index, never LinkedIn, counted
against the month's budget. When the index has no copy under the link,
the read asks Exa's people search once more by the name in the link,
like "erika beispiel" for `/in/erika-beispiel-71`, since the index often
holds a profile under another link, like de.linkedin.com. That is one
more search, and only a profile under the same link name counts, never a
namesake. A link with no name in it asks nothing more.

`PUT /api/people/{id}/profile` with `{"text": "…"}` keeps a LinkedIn
profile page Tim copied in Chrome, all of it with ⌘A and ⌘C, as the
person's profile, for someone Exa's index lacks. The app never requests
LinkedIn: Tim opened the page. The name is the line that spells the name
in the link, also without initials or with a title or more around it,
the headline the line after it, the city from the first line after it
in the shape of a place, like "Köln, Nordrhein-Westfalen, Deutschland".
The page is read by its sections: the top, then About, Experience,
Education and the rest, and the feed of posts, Activity, last, so the
8000 characters it is cut at, like Exa's text, go to who they are first.
LinkedIn's menu and buttons, and its blocks of other people, like
"People who follow … also follow", Highlights, Interests and "People
also viewed", are left out, so are email addresses and phone numbers. A read of them Exa is still asked for loses to
it, and a run that had not begun it drops it. It answers `{"person": {…},
"named": true}`: `person` as `GET /api/people/{id}` gives it, and
`named` whether the name on the page is the one in their link, which
makes sure the page is theirs. A page whose name is not is read all the
same, its name the line LinkedIn puts above "· 1st". It is a step that
`POST /api/undo` takes back, see [Undo](#undo). 400 for a paste with no
text, 404 for an id nobody has.

`DELETE /api/people/{id}` takes out again someone added from a link, for
Undo, and answers 204. A run that had not read their profile leaves it,
one that read it keeps the read. 409 for someone a search found, 404 for
an id nobody has.

`GET /api/people/{id}` gives one person, with `notes`, `fit_evidence` and
`sightings`: each search that found the person, newest first. `draft` is
the first message to them, empty before the first draft, and
`drafted_at` when the language model wrote it. `profile_read_at` is when
Exa's index first gave the profile text a draft is written from as it is
now, or Tim pasted it, null while the
app has none and a draft reads it first. See [Drafts](#drafts).
`fit_parts` are what `fit_score` adds up, in the order the person's Why
shows them, each with its points and why, like
`{"key": "near", "label": "Near you", "points": 13, "why": "Bonn, about 24 km"}`.
The keys are `who`, `active`, `near` and `younger`, in that order. Only
`who` can be below 0, by signals against, never by more than the others
add up to.
Each part has `lines`, what the person shows under it, each
`{"mark": "for" | "against" | "neither", "text", "passage", "found_in"}`:
under `who` the signals with their passages, under the others the part's
why, marked by how much of the part it gave.

`PATCH /api/people/{id}` with `{"connected": true}`, `{"connected": false}`,
`{"notes": "..."}` or `{"draft": "..."}` answers the person as `GET`
does. An empty `draft` clears `drafted_at` too.

`PATCH /api/profiles/{id}` with `{"review": "confirmed"}`, `"rejected"` or
`"open"`. A person has one profile, so its id is the person's.

Each of these is a step that `POST /api/undo` takes back, see
[Undo](#undo), and so is the first message the language model writes
after Draft.

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

`POST /api/runs` starts a run of the due searches, and of the profiles
of people added from links that a run before did not read, and answers
`{"run_id": 3, "started": true}`. When a run already goes, `started` is
false and `run_id` is that run. Without an Exa key, with the key kept
back by macOS because its password box was closed, with the month's
budget spent, or with no search due it gets 409 and says which.

`GET /api/runs/current` answers `{"run": null}` or the going run, with
`progress`: how many searches it asks, `checks` and `checks_done`, how
many profiles it reads, `reads` and `reads_done`, which are being asked or
read now and since when, and `seconds_left`, measured from the last 200
searches asked and the last 200 profiles read. `seconds_left` is null until there is something to
measure.

`POST /api/runs/{id}/stop` stops a going run and answers 204, or 409 when
that run is not going.

`GET /api/runs` lists the last 50 runs, newest first. `GET /api/runs/{id}`
gives one, with its `checks` and its `failures`.

A run's `state` is `going`, `finished`, `stopped`, or `interrupted` when
the app was closed while it went. A run started by adding links has the `kind`
`add`. Stopped, the profiles it did not read wait for the next run.
`reads_failed` counts the profiles Exa could not give in it. One not in
Exa's index yet is no failure, its person says so.

## Settings and the key

`GET /api/settings` lists every setting with its value and what it
does. `hook_prompt` is the prompt the language model writes the hook of a
first message from, see Drafts, and must keep `{{profile}}`. It was
`first_message_prompt` while the model wrote the whole message, and a
prompt kept under that name is dropped when the file is read. `home_city`
is Where you are, the city in NRW the fit measures distances from, empty
until Tim sets it. It takes a city the app knows in any spelling, like
`"Cologne"`, and keeps its German name, `"Köln"`. Any other place gets
400. Where LinkedIn opens is not one of them, it has its own endpoints
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

## Copy

`POST /api/clipboard` with `{"text": "…"}` puts the text on the Mac's
clipboard and answers 204. It is answered by the app itself, not by
`internal/api`, see `cmd/speakertrail-app/clipboard.go`. The window's own
`navigator.clipboard.writeText` is refused in the app's web view with a
`NotAllowedError`, so the copy mark
on a first message and Copy on the Updates page ask the app instead. When
the Mac does not take it, the answer is 500 with an `error` that says to
select the text and press Cmd+C.

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
| `POST /api/language/use` | `{"name": "…"}` picks the model that drafts, downloaded or not, and answers in the shape of `GET /api/language`. 409 with why for a model that is not offered |
| `DELETE /api/language/models/{name}` | takes that model off the Mac, after Settings asked. Not while a draft writes with it. Removing the model picked lets the choice go |

A model FrameFairy has whole in `~/.framefairy/models`, by its size and
checksum, is linked from there rather than downloaded, so the two apps
share it on disk. While it is checked, `progress.text` is `copying`, with
the share read so far and the seconds left as a download has them, and
Cancel stops it. Such a model is `shared` from then on. A download that
arrived whole before it was stopped is read through again the same way,
with `progress.text` `checking`, rather than fetched again. The speech
models do the same.

`loaded` marks the model in memory now. The model that drafts is loaded
as the app starts, after it is downloaded and after it is picked, and
stays until the app quits, so every draft writes at once. The event
`language` says when it is in.

`standing` is whether a draft can be written now, `{"mark": "ok", "why": ""}`,
and what stands in the way when it cannot: `warn` when the model that
drafts is not downloaded, `err` when this build has no llama-server or
the model needs more memory than this Mac has. `why` says it in a
sentence. Settings, the dot on Settings in the sidebar and Draft all show
this, and none of them works it out alone.

`needs` is the memory the model takes while it writes, in bytes, and
`fit` what this Mac can do with it: `fits`, `tight`, `too big` or
`unknown`. `recommended` marks the largest model this Mac holds
comfortably, the one Settings offers. `inUse` marks the one that drafts:
the one picked in Settings, kept in the app's file as `language_model`,
here or not, and with none picked the largest on this Mac. While the one
picked is not downloaded, a draft is refused with 409 rather than
written with another. `job` is in the shape of a speech model's
download.

Where a download stands is sent as the Wails event `language`, in the
same shape as `GET /api/language`.

## Drafts

A draft is a first message to a person, put together on the Mac from the
template in Settings, with one variation of each part picked at random.
The language model writes only its hook, the parts `portrait`, `reason`
and `reaction_part`, from their profile and the prompt in Settings,
`hook_prompt`. It answers in JSON held to `draft.HookSchema`, which
llama-server turns into a grammar and the app checks again. The prompt
goes in two turns: the paragraphs before the one of its first field as a
system turn, the same for every draft, and the rest, filled in for the
person, as the user's. llama-server keeps a checkpoint where the last
user turn starts, so the next draft reads only the person's part again. See
`internal/draft`. Drafts are written one at a time, in the order they
were asked for.

| Request | What it does |
| --- | --- |
| `GET /api/drafts` | the latest draft of each person since the app started, oldest first |
| `POST /api/people/{id}/draft` | `{}` starts a draft for them and answers it at once. Asked again while theirs is under way, it answers that one. 409 with why when no language model is downloaded, and nothing starts |
| `POST /api/people/{id}/draft/cancel` | `{}` stops theirs, 204. 409 when none is under way |

A draft is

```json
{ "id": "draft-3", "person_id": 7, "state": "writing", "model": "Gemma 4 26B A4B", "text": "Hallo Lea, danke", "started_at": "2026-11-19T14:00:00Z", "fraction": 0.6, "seconds_left": 4, "seq": 12 }
```

`state` is `queued`, `reading` (their profile from Exa, when the app has
no text of it yet, by the link and then by the name in it, like a run's
read: a draft whose profile Exa lacks fails and says to paste it, see
`PUT /api/people/{id}/profile`), `loading` (the model, when it is not loaded), `writing`,
`done`, `failed` or `cancelled`. `text` is the message, from `writing` on
with `…` where the model's parts go, and whole once it is done. What the
model wrote in it is untrusted and shown as text. `problem` is set on a
draft that is done when the model's answer did not fit the schema: the
message then has `…` where the model's parts go, for Tim to write, and
`problem` says why in a sentence, never with the answer itself.
`fraction` and `seconds_left` come from how long earlier drafts took, -1
and null before there was one. `error` says why a draft failed, in a
sentence that can go on screen. `seq` is higher for every change than for
the one before it, of any draft, so the window keeps the newer of the
answer to `POST` and the event, whichever arrives last.

A draft that is done is kept on the person as `draft`. Reading a profile
counts against Exa's monthly budget like a search. A draft that needs
the Exa key fails with why when macOS keeps it back, and Draft again
asks macOS again. Each change is sent as
the Wails event `draft`, one draft at a time, and once a second while one
runs. Where the app runs no language model, starting one answers 409.

## First message template

The template a first message is put together from, as Tracery does it:
each part has variations, a draft starts at `origin` and picks one
variation of each part it meets at random, and `#name#` inside a
variation stands for a variation of the part `name`. An empty variation
leaves the part out now and then, a part with no variation at all leaves
it out always, and a variation written twice is picked twice as often. The app fills `#first_name#`, the language model `#portrait#`,
`#reason#` and `#reaction_part#`, a space and the reaction or nothing.
What they fill in is put in as it is, never read for `#name#` again.
Tim's first template is `internal/store/first_message_template.json`.

| Request | What it does |
| --- | --- |
| `GET /api/template` | the template, below |
| `PUT /api/template/parts/{name}` | `{"variations": ["Hallo", ""], "at": 2}` gives the part its variations, or adds it at `at`, at the end without one. A part that is there keeps its place |
| `DELETE /api/template/parts/{name}` | takes the part out. 404 for a part that is not there |
| `DELETE /api/template` | puts Tim's first template back |
| `PUT /api/template` | `{"parts": [...]}` puts a whole template in place, the one before a reset when it is undone |
| `GET /api/template/example?seed=7` | `{"text": "Hallo [first_name], ..."}`, one message put together at random, with what the app and the model fill in shown by name. The same seed makes the same picks, so Settings keeps its example while the text of a variation changes. Adding or removing one, or a `#part#`, can change the picks after it. Without a seed the picks are new each time |

Each change answers with the template:

```json
{ "parts": [{ "name": "origin", "variations": ["#greeting# #first_name#, ... #reason##reaction_part# ..."],
             "kept": "Origin is where the message starts, so it stays." }],
  "fields": [{ "name": "reason", "by": "model", "about": "Why they caught Tim's eye, the part after \"weil\"" }],
  "changed": true }
```

A change that leaves a template that cannot work answers 400 with why,
and changes nothing: a part named other than with small letters, digits
and `_`, or with the name of a field, `#name#` for a part there is not,
an invisible character, like a zero width space or a mark that turns text
right to left, where a control character other than a line break, the
joiner inside an emoji aside, counts as one, a part inside itself or more than 20 deep, no
`origin`, no `#reason#` that origin reaches, a longest message of more
than 3,000 letters, or origin taken out. Each part is walked once, so the
check takes as long as the template is. `kept` says why a part cannot be
taken out, origin or a part another part uses, and is missing when it
can. `changed` says the template is not Tim's first one any more.

## Report a Problem

`/api/report` is answered by the app itself, not by `internal/api`. It
keeps the draft of a report in `~/Library/Application Support/Speaker
Trail/report`, so a draft outlives the window and the app. See
`cmd/speakertrail-app/report_api.go`, `bugs.go` and `report.go`. Errors
answer 409 with a sentence that can go in a toast.

| Request | What it does |
| --- | --- |
| `GET /api/report` | the draft, `{"text": "…", "pictures": [{"id": "p1", "name": "Screenshot….png", "width": 1440, "height": 900}], "people": false}`. Pictures removed a moment ago are left out |
| `GET /api/report/window` | `{"open": true}` while the report window is open |
| `POST /api/report/open` | `{"how": "bugs"}` opens the report window, or brings it to the front |
| `POST /api/report/about` | `{"text": "…"}` opens it with the words of a toast after what was written |
| `PUT /api/report/text` | `{"text": "…"}` keeps what was written |
| `GET /api/report/picture?id=p1` | the picture itself, as PNG |
| `POST /api/report/pick` | asks for pictures in a Finder dialog, PNG or JPEG |
| `POST /api/report/pasted` | `{"name": "…", "data": "<base64>"}` adds a pasted picture |
| `POST /api/report/remove` | `{"id": "p1"}` takes a picture out, and keeps it aside for Put it back |
| `POST /api/report/put-back` | `{"id": "p1"}` puts it back |
| `POST /api/report/clear` | empties the draft, and keeps it aside for Undo. People stays as it is |
| `PUT /api/report/people` | `{"people": true}` takes People along, and answers the draft. Every new draft starts with it off |
| `POST /api/report/send` | `{"what": "…"}` makes the report, with `people.json` when the draft takes People, a zip on the Desktop, shows it in Finder, closes the window and answers `{"path": "~/Desktop/Speaker Trail report 2026-11-16 10.15.zip"}` |

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

Edit, Undo (Cmd+Z) and Redo (Cmd+Y) in the app's menu do not reach the
page as keys. The app sends the Wails event `undo`, `"undo"` or
`"redo"`, which reaches every window, and only the page that has the
focus takes it. It decides: Report a Problem undoes its
own steps, a text field anywhere undoes what was typed, and anywhere
else the app takes back the last change Tim made. See
`cmd/speakertrail-app/menu.go` and `internal/store/history.go`.

`POST /api/undo` takes back the last change Tim made, and `POST
/api/redo` does again what the last undo took back. A change is one
step: Connect, the notes, the first message, Draft, Not this person and
a pasted profile, each on one person. A change that changed nothing is
no step, and neither is anything a run does. Steps are kept while the
app is open, up to 200, and a new step empties Redo. They answer what
they changed, so the page opens it, or `done` false when there was
nothing to take:

```json
{ "done": true, "shows": { "kind": "person", "id": 7 } }
```

An undo puts back only the fields the step changed, so what a run
changed since stays. When one of those fields changed again since, it
writes nothing and answers 409, "This has changed since, so it cannot be
taken back", and every step behind it in the same list goes, since they
rest on it.

Searches, exclusions, the template, the settings and the people a run
added follow, see issue #71. Until then they keep their Undo toasts.
