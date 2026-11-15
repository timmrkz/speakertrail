# The app's log

How the app keeps a record of what it did, so that a fault on Tim's Mac is
found by reading what happened, not by asking him to try again. Ported
from FrameFairy's log, which works the same way, and tracked in issue #16.
The report that takes it along is #11.

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
| `from` | the part that wrote it: `app`, `api`, `engine`, `updates`, `wails`, `window` |
| `msg` | what happened, in words |

Other fields come where they mean something: `run` and `search` for the
run and the search a line is about, by id, `ms` for how long a check
took, `profiles`, `found` and `new` for what it found, counted and never
named, `path` for a request,
`error`, `stack`, and `build`, `channel`, `commit`, `os` and `arch` when
the app starts.

FrameFairy calls the start of the app `run`. Here a run is a run of
searches, so the start is `start`, and `run` is the run's id.

No line holds a person's name, headline, profile link or notes, nor the
Exa key. A person is named by id.

### Levels

| Level | What it is for |
| --- | --- |
| `error` | something failed: a run that could not be saved, a request that failed, a panic |
| `warn` | something went wrong and the app carried on: a search that failed, a check for updates that failed |
| `info` | the app starting, a run starting and ending, a channel picked |
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
# the lines of the last start of the app
jq -c --arg s "$(tail -n 1 app.log | jq -r .start)" 'select(.start == $s)' app.log
```

## The steps

1. Done. zerolog and lumberjack in place of slog on the standard error:
   the place, the cap, JSON lines, the levels, `start` and `from`, and
   Wails's, the engine's and the API's lines in the same file. Panics,
   the standard log, and the rule in `.golangci.yml`.
2. The window's lines, `web/src/lib/said.ts` as in FrameFairy: its errors,
   calls slower than 150 ms, and acts, each click and key with an id that
   every line it sets off carries.
3. Settings, Logs: where the log is, how much it holds, Show in Finder,
   remove after asking, and the last five toasts.
