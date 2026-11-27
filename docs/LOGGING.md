# The app's log

How the app keeps a record of what it did, so that a fault on Tim's Mac is
found by reading what happened, not by asking him to try again. Ported
from FrameFairy's log, which works the same way, and tracked in issue #16.
Report a Problem takes it along, see below.

## Where it is

`~/Library/Logs/Speaker Trail/app.log`, the place Mac apps keep their
logs, which the Console app shows. Elsewhere, in a cloud session or on a
build runner, it is in the user's cache folder,
`~/.cache/SpeakerTrail/logs/app.log` on Linux.

Every line, the steps included, is always written, and what keeps the
disk free is a cap, not a switch. The log is at most 10 MB of lines in
use and three older files compressed beside it, a few MB more. That holds
many hours of work. It is never more, whatever happens.

The detail has to be there already when the fault happens. A log that
writes errors only, with the steps behind a switch, would have to be
switched on, the fault made to happen again and the log sent, which is
the round trip this log is there to save. FrameFairy had a Detailed Log
switch once, and Tim took it out: a switch that changes nothing on screen
is one nobody can judge, and the one moment it is needed is the moment it
is too late to tick it.

## The library

[zerolog](https://github.com/rs/zerolog) writes the lines, and
[lumberjack](https://github.com/natefinch/lumberjack) keeps the file to
its size. Both are MIT licensed and widely used. Nothing of our own does
either job.

| | zerolog | zap | log/slog |
| --- | --- | --- | --- |
| Speed and allocations | fastest, none per line | close behind, few | slower, some per line |
| Fields on every line of a part | `With()` | `With()` | `With()` |
| Levels | trace to panic | debug to fatal | debug to error |
| Takes lines from code that speaks slog | `zerolog.NewSlogHandler` | `zapslog` | is it |

zerolog it is, as in FrameFairy. It allocates nothing per line, has a
trace level below debug, and takes slog's lines. Wails, which runs the
app's window, logs through slog, and so do the engine and the API. Their
lines go into the same file that way: Wails's from `info` up, since its
debug lines are one for every request the window makes, and the engine's
and the API's from `trace` up.

A panic Wails catches is an `error` line from `wails` with its `stack`,
and the app goes on. A panic in a request of the window, which Wails does
not recover, fails that request and is an `error` line from `api` with
its `path` and `stack`.

What a library or net/http says through Go's standard log is a `warn`
line from `app`, since nobody reads the standard error of a Mac app
opened from Finder. Our own code never writes there, which the rule in
`.golangci.yml` holds and `make lint` checks.

## What a line holds

Each line is one JSON object, so it can be filtered by any field rather
than read with the eye:

```json
{"level":"info","start":"9f3a01","from":"engine","run":12,"searches":10,"time":"2026-11-14T18:41:07.512+01:00","msg":"a run started"}
```

| Field | What it is |
| --- | --- |
| `time` | the moment, to the millisecond, with the Mac's offset from UTC |
| `level` | `error`, `warn`, `info`, `debug` or `trace` |
| `start` | one start of the app, so a restart is seen and two starts never mix |
| `from` | the part that wrote it: `app`, `api`, `engine`, `keys`, `updates`, `wails`, `window` |
| `msg` | what happened, in words |

Other fields come where they mean something: `act` for what the person
did that led to it, see Acts below, `toast` for a toast about something
that went wrong, `run` and `search` for the
run and the search a line is about, by id, `ms` for how long a check
took, `profiles`, `found` and `new` for what it found, counted and never
named, `path` for a request,
`error`, `stack`, and `build`, `channel`, `commit`, `os` and `arch` when
the app starts.

FrameFairy calls the start of the app `run`. Here a run is a run of
searches, so the start is `start`, and `run` is the run's id.

No line holds a person's name, headline, profile link or notes, nor the
Exa key. A person is named by id.

The Exa key lives in its own file, and reading it logs nothing. Moving a
key an older build kept in the keychain is a line from `keys`, `the Exa
key was read from the keychain, to move it to its file`, with `ms`,
whether it was `found` and the keychain's `status`, `0` when it was and
`-128` when the password box was closed. `ms` shows how long the box was
on screen. Then `the Exa key was moved from the keychain to its file`,
or a warning that macOS kept it back. A delete macOS refuses is a
warning with its `status`. This happens once, and the keychain is never
read again.

### Acts

What the person did is the thread a fault is followed by. Each act is a
line at `info` from `window` with a short id of its own, `a1`, `a2` and
on: every click, and every key that is not typing. It is named by the
control it was on, by its `aria-label`, its tooltip or its words, as in
FrameFairy: `pressed button.btn "Run the due searches now. Takes about a
minute"`, `⌘F on input.input "Search settings"`. Everything that act sets off carries its id, in the window and
on the Go side: the requests it makes, Wails's, the engine's and the
API's lines. The window tells the Go side of each act the moment it
happens, before the requests it makes for it, and the Go side's lines
carry the act it was told of last. A run keeps the act that started it
on every line it writes, however long it goes and however many acts
follow, the way a job does in FrameFairy.

An act never names a person. Whatever shows a person's words is marked
`data-personal`: the rows of People, the person's view and the sheet it
slides in on, the first message among them. A draft is logged by the
person's id and its counts, how long it took and how much the model read
and wrote, never by the profile, the prompt or the message. An answer of
the model that did not fit the schema is logged with what did not fit
and which of the two asks it was, never with the answer. The counts of
a draft add up both asks. A control
there is named by its kind, `pressed a.prow`, or,
for a button, by its own words, which are ours, `pressed button.btn
"Connect"`. FrameFairy names a word clicked in its caption box as the
caption box the same way.

### What the window says

`web/src/lib/said.ts`, as in FrameFairy, gathers the window's lines and
sends them within a quarter of a second, at most 400 at a time, to
`POST /api/log/said`. A line said again straight after itself is counted,
`(said 3 more times)`. The Go side takes at most 400 lines of at most
2000 characters from one request, and keeps the window's moment for each
unless the window's clock is more than a minute off.

- Errors and warnings of the console, what is thrown and not caught, and
  promises rejected with nobody listening, at `error` or `warn`.
- `window: started`, with the browser engine, the pixel ratio and the
  window's size, and `window: hidden` and `shown`.
- A request to the Go side slower than 150 ms, `calls: GET /api/people
  took 212 ms`, one that failed with why, and more than 60 in one
  second, which is a loop gone wrong. A request is named by its method
  and path, never its query, which holds what was typed in a search.
- An act that took over 150 ms from the hand to the first frame drawn
  after it, at `warn`, with how much of that time the window was still
  busy with something else before it heard the act.
- Every toast. One about something that went wrong is a `warn` line
  marked `"toast":true`. One that says what was done is an `info` line.
  A toast that ran out, was replaced or acted on says so at `info`.

### Settings, Logs

Settings shows the log in an area of its own, Logs: where it is, how many
files with the older ones, their size and their lines. While Settings is
open the app sends it again as lines are written, as the Wails event
`applog`, at most once a second. Each file is read once, and the file in
use from where it was read last, so keeping up costs the lines written
since. The folder button, Show in folder, shows the log in Finder. The
trash can removes every file of it after asking, and the next line starts
a new log that says how many files, bytes and lines went.

Under it are the last five toasts about something that went wrong, read
back from the lines marked `"toast":true`, newest first, a toast said
again once, where it was said last, so one that ran out unread is not
lost. Each has Report beside it, as in FrameFairy, which opens Report a
Problem with the toast's words in it. So does a toast on screen about a
failure Tim cannot put right himself, such as a person that could not be
saved, as FrameFairy marks one. A missing key or a spent budget is his to
put right and offers none.

### Levels

| Level | What it is for |
| --- | --- |
| `error` | something failed: a run that could not be saved, a request that failed, a panic, an error in the window |
| `warn` | something went wrong and the app carried on: a search that failed, a check for updates that failed, a request slower than 150 ms, an act slower than 150 ms to reach the screen, a toast about something that went wrong |
| `info` | the acts, the app starting, a run starting and ending, a channel picked, a toast that said what was done |
| `debug` | the steps between: every check of a run, a search stopped, the title bar measured |
| `trace` | lines that come with every step of something that repeats |

Everything from `debug` up is always written. `trace` lines go to memory
instead of the file, the last 8000, a few MB at most, for the report to
take along.

## Reading it

The log is JSON lines, read with [jq](https://jqlang.org). For example:

```sh
# every error and warning
jq -c 'select(.level == "error" or .level == "warn")' app.log
# everything one run of searches did
jq -c 'select(.run == 12)' app.log
# everything one act set off
jq -c 'select(.act == "a41")' app.log
# what took long enough to be felt
jq -c 'select(.msg | test("reach the screen|took [0-9]+ ms"))' app.log
# the lines of the last start of the app
jq -c --arg s "$(tail -n 1 app.log | jq -r .start)" 'select(.start == $s)' app.log
```

## Report a Problem

Bugs in the sidebar opens Report a Problem in a window of its own. Tim
writes or dictates what happened and attaches screenshots and screen
recordings, and Report
puts one zip on the Desktop, `Speaker Trail report 2026-11-16 10.15.zip`,
and shows it in Finder. In it:

| File | What it is |
| --- | --- |
| `what-happened.txt` | what Tim wrote, when he wrote anything |
| `about.json` | when it was made, the build, `start`, macOS, the chip and the memory |
| `settings.json` | the settings Settings shows, but for Where you are, which says where Tim lives. Never the people, the searches, the notes or a key |
| `first-message-template.json` | the first message template as Settings has it, in the shape of `internal/store/first_message_template.json` |
| `people.json` | only with People on, see below |
| `logs/` | every file of the log, the older ones unpacked, and `recent.log`, the `trace` lines kept in memory |
| `pictures/` | the screenshots, as PNG |
| `recordings/` | the screen recordings, `1.mov` and on, byte for byte as they were attached, so with whatever their files say about themselves, a place or a device too |

Paths under the home folder are written as `~` in every file but the
pictures and recordings, so the Mac's user name does not travel. The report never holds
the app's file.

### People

The People button beside Attach takes People along, so Claude can read
the data behind what People shows without screenshots, and score a
profile again from its text. Tim asked for it on 22 November 2026, issue
#73, and chose what goes: `people.json` holds every person by id, with
what the person screen shows and the profile text. That is the headline,
the city, the roles, the age guess and why, when they were last active,
the rubric's signals with their passages, the fit as People orders by it
on the day of the report with each part's points and why, the newest
post or comment the profile text shows, whether Tim connected with
them, whether the profile is confirmed or rejected, the exclusion that
leaves them out, when they were found and by which searches, and whether
a first message was drafted. Never the name, the profile link, the
handle, the notes or the first message. Inside every text that goes,
each word of the person's own name of three letters or more that starts
with a capital, also with an s after it and with its umlauts written
out, and the handle are written `[name]`, and every link `[link]`. "von"
and "der" stay. Other people's names in a profile text go along, which
Tim accepted. The names are taken out after the store's lock is let go,
since that reads every profile text, see `Store.Share`. `store.Shared` lists what
goes, so a field added to a person goes only once it is added there.

People is off in every new draft and goes off again once a report is
made, so it goes only in a report it was switched on for. The draft on
the Go side says whether it goes, and Report waits for a press of People
still on its way. Clear leaves it as it is. The draft is kept in `~/Library/Application
Support/Speaker Trail/report` until Report is pressed, so closing the
window or the app loses nothing.

Dictate writes what Tim says into the field, on this Mac, with the speech
model from Settings, Speech models. macOS asks for the microphone the
first time. A recording goes on for as long as Tim talks, and stops by
itself after two minutes without a word, issue #58.

## The steps

1. Done in #21. zerolog and lumberjack in place of slog on the standard
   error: the place, the cap, JSON lines, the levels, `start` and `from`,
   and Wails's, the engine's and the API's lines in the same file.
   Panics, the standard log, the rule in `.golangci.yml`, and every check
   of a run.
2. Done. The window's lines and acts, `web/src/lib/said.ts`, over
   `/api/log` where FrameFairy has Wails bindings, see
   [api.md](api.md).
3. Done. Settings, Logs.
4. Done in #24. Report a Problem takes the log along.
