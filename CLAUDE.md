# CLAUDE.md

Guidance for Claude when working on this repository. Read it fully before the
first change of a session. The user is Tim.

## The project

Speaker Trail finds people in North Rhine-Westphalia who run their own
business and could be guests on Tim's podcast, *My First Memory*. It is a
Mac app, built like Tim's FrameFairy: Go with Wails v3 for the window, the
Svelte interface built in, and everything it knows in one JSON file in
`~/Library/Application Support/Speaker Trail`. Runs ask profile searches
through Exa. No Docker, no database, no server.

The public event calendar and the crawler of event pages are paused. Their
code still sits in the repository and the app does not use it.

## Where things are

Start with [README.md](README.md). In short:

| Part | Code | Docs |
| --- | --- | --- |
| the app, its window | `cmd/speakertrail-app/` | |
| the file: people, searches, runs, settings | `internal/store/` | |
| runs of profile searches | `internal/engine/` | |
| HTTP API the interface talks to | `internal/api/` | [docs/api.md](docs/api.md) |
| the Exa key in the keychain | `internal/keys/` | |
| interface, Svelte 5 | `web/` | `.claude/skills/interface/` |
| build, the .app bundle | `Makefile`, `scripts/` | [docs/BUILD.md](docs/BUILD.md) |
| who to look for, and where, the fit rubric | `internal/rubric/` | [docs/search-strategy.md](docs/search-strategy.md) |
| working with Claude | | [docs/WORKFLOW.md](docs/WORKFLOW.md) |
| paused: server, crawler, queue, Postgres, Docker, deploy | `cmd/speakertrail/`, `internal/server/` and the rest | [docs/server-api.md](docs/server-api.md), `.claude/skills/docker/` |

## How Tim works

- He uses Claude through claude.ai, in chat and in cloud sessions at
  claude.ai/code. Changes reach him as pull requests on GitHub. After a
  merge he runs `git pull && make run`. Never hand him zip files or ask him
  to copy files around.
- **Every piece of work becomes a pull request, always.** A branch he has to
  find himself is work he cannot see.
- **The pull request comes first, not last.** Push the first commit and open
  the pull request straight away, without being asked, and keep pushing to
  it. The first commit does not have to be worth looking at. It exists so
  the pull request exists, because that is where Tim follows the work.
- His machine is an M2 Max with 32 GB of memory, on the latest macOS. What
  the build needs, Go and Node.js, make installs with Homebrew, as it does
  for FrameFairy. Nothing beyond that, and never as a step he has to do.
- He tests the app himself on his Mac. After each change say exactly what
  to look at and what should happen.
- He answers in English or German. Reply in the language of his message.
- Measurements are metric.

## How to work with him

- **Think first.** Research and check before answering. Read the code and
  docs you are about to change. Verify facts about tools, versions and
  websites instead of assuming them.
- **Small batches.** Split larger work into batches of a few minutes each.
  Finish, test and report after each one. Never disappear into a long
  stretch of work.
- **Just do small things.** Small, low-risk changes need no discussion of
  internal choices. Discuss constraints only for security, lasting effects,
  personal data, or side effects he would not expect.
- **Ask at most one question at a time**, and only when the answer changes
  what gets built.
- **Report plainly.** What changed, what to test and how, what is still open.
  Name bugs you found on the way, including your own.
- **Watch a pull request, quietly.** Subscribe to its events and act on
  them: a failing CI run is still your work, so is a review comment. But
  never set up a recurring check, never poll, and never write a message that
  says nothing happened. Tim comes back when he is ready.

## Writing rules

These apply to everything written for Tim: interface text, docs, commit
messages, pull request text, code comments and chat replies.

- **Never use semicolons.** Use commas and full stops. Semicolons in code
  syntax are fine, in prose and comments they are not.
- Plain, direct language. Short sentences.
- **One name per thing.** A **source** is a page the crawler checks again
  and again. A **check** is one look at one source. A **read** is the
  language model's look at one event's own page, for the people on stage.
  A **portfolio** is a source that lists startups, a **directory** one that
  lists businesses run by people, like gyms or coaches, and a **lookup** is
  one look at one startup's or business's website, its imprint and its
  about page, for who runs it.
  A **search** is a source that asks a search provider, like "BJJ Gym
  Köln", and its results are websites to look up.
  A **post search** is a source that asks Exa for LinkedIn posts, like
  "Ein Jahr selbstständig #köln", and its results are the people in NRW
  who wrote them.
  A **profile search** is a source that asks Exa for LinkedIn profiles,
  like "Yogalehrerin Köln", and keeps the people in NRW who were active
  lately and run their own business. Runs take only these for now.
  A **run** is one pass over the due sources, nightly or started by hand. A
  link to one event, added as a source, becomes the calendar it belongs to.
  Seeds exist only in the starting data, never in the interface. An
  **event** is kept or dropped, never accepted or rejected. A **person** is
  kept or skipped. A **profile** is confirmed or rejected. The public page is the **calendar**, Tim's side is
  the **workspace**. Whatever a thing is called in the interface, it is
  called that in the docs and the code comments too.
- Docs live in `docs/`. Only `README.md` and this file sit at the top, and
  the deployment guide sits beside what it deploys, in `deploy/`.
- When behaviour changes, update the matching doc in the same change.

## Interface rules

- **Narrow first.** The window goes down to 390 px wide. Every screen works
  there with no sideways scrolling, and the first view of every screen
  answers its main question without scrolling.
- **The interface is the mechanism, not the machinery.** Every control has
  to earn its place by serving what the person is there to do. What the
  engine needs in order to work is not a feature. Whatever a person can do
  with one click has to be undoable as easily.
- **Help waits until it is asked for.** No line of instructions sits on
  screen for good. What a control is for goes in its `title` or behind a
  small info mark.
- **A row never changes width as you use it.** A label that toggles gets
  room for the longer word, so nothing wraps and nothing jumps.
- **Controls in one row have the same size.** No one-off sizes.
- **What cannot be taken back asks first.** Everything else saves at once
  and is undone with a click. No save buttons, except a form that adds
  something new.
- **A click shows at once.** A control that starts work says what it is
  doing and takes no second click.
- **Work never runs silently.** Anything that takes more than a moment
  says what it does now, how far it is, how long it has run and about how
  long is left, measured from earlier work rather than guessed, and it can
  be stopped. It looks the same everywhere: the fill, the shuttle and the
  pulsing dot in `app.css`, and `RunProgress.svelte` for a run. See the
  interface skill.
- **Short runs, often.** Tim runs the engine by hand on his Mac. A run
  stays short enough to watch, and what does not fit waits for the next.
- **Consistency over novelty.** A visual treatment used in one place is used
  for every equivalent element, or not at all. Two things of the same kind
  are the same size, in the same colour, in the same place.
- Colours and sizes come from the tokens in `web/src/app.css`, in both
  themes. Tabular figures for times and counts.

## Engineering rules

- **Go, latest version.** One module, `go 1.27` in `go.mod`. Upgrade with Go
  releases.
- **Built like FrameFairy.** A native Mac app with Wails v3, no Docker, no
  database process, no server. What the app knows lives in one JSON file,
  written whole to a new file that then replaces the old one. Secrets live
  in the macOS keychain, never in a file.
- **One command.** `make run` goes from a fresh clone to a running app.
  Nobody should have to remember a second command. On a Mac make installs
  what is missing with Homebrew, `scripts/tools.sh`, and nothing else ever
  installs anything. Never tell Tim to `brew install` or `npm install -g`
  anything himself. With `CI` set make installs nothing.
- **It builds on Linux too.** Build runners and cloud sessions build the
  window against GTK 4 and WebKitGTK 6 and run every test there. CI's
  `macos` job proves the Mac build, the bundle and the keychain.
- **No LinkedIn, no Instagram.** The engine never requests them, not even
  for robots.txt, and the headless browser goes through a filter that
  refuses them. `TestNeverRequestsLinkedInOrInstagram` and
  `TestBrowserNeverReachesBlockedHosts` prove it. Never weaken either.
- **Polite crawling.** robots.txt is respected, the bot names itself, one
  request every 5 seconds per website. A site that refuses the bot becomes
  a manual source. No workarounds.
- **Personal data.** Names, roles and profile links of speakers are personal
  data. Store only what the data model lists, never email addresses or phone
  numbers. Names stay out of the public calendar unless
  `public_show_people` is on. Real names never go into test fixtures, mock
  data or anything committed: invent them.
- **Untrusted input.** Every crawled page, feed and pasted link is
  untrusted. Stored pages are served as sandboxed plain text, never as HTML.
- **What runs on its own goroutine is tested from several at once.** Every
  Go test runs under the race detector, and anything asynchronous, the
  queue above all, has a test that uses it from several goroutines at once.
  A fix starts as a test that fails on the code before it.
- **Tests** need no network and never touch a live website. Pages go in
  `testdata/`, with invented people. Run `make test` before every push that
  touches Go. A change only to `web/` or `docs/` runs `make interface`.
- **Short updates.** The file is written on every change, so nothing
  slow, above all no call to Exa, happens inside `Store.Update`.
- `gofmt` and `go vet` must pass.
- The interface is built by make into `web/dist/`, which Go embeds. Only
  `web/dist/.keep` is in the repository. Never commit build output, `.env`
  or the app's file.
- **No paid language model.** The paused crawler read pages with a local
  model through Docker Model Runner. Whatever comes back needs no paid
  model API, no key and no credit card.
- Every paid API gets a monthly budget in Settings, and the engine stops
  calling it once the budget is reached.

## Cloud sessions

Sessions at claude.ai/code run on Ubuntu 24.04 with the setup script from
`scripts/cloud-setup.sh`, which provides Go 1.27, GTK 4 and WebKitGTK 6 for
the window, and a local Postgres 16 for the paused server's tests.
`scripts/session-start.sh` starts Postgres and sets `DATABASE_URL` and
`TEST_DATABASE_URL` in every session.

- If `go version` does not show 1.27, or `pkg-config --exists gtk4
  webkitgtk-6.0` fails, run `bash scripts/cloud-setup.sh`.
- `make test` and `make` before pushing anything that touches Go.
- The network is limited to an allowlist set in the environment. Exa may
  not be on it, and tests never call it anyway.
- There is no screen, and the window cannot open. Interface work goes
  through the skill in `.claude/skills/interface/`. Read it before changing
  anything in `web/`.

## Where guidance goes

When Tim says how he wants something done, write it down in the right
place, in the same pull request.

- **This file** holds what every session needs before its first change:
  who Tim is, how he works, the rules that are never broken, and the words
  things are called. Keep it short, because it is read in full every time.
  One line per rule, with the reason when it is not obvious.
- **A skill**, in `.claude/skills/NAME/SKILL.md`, holds how to do one kind
  of task: steps, tools, what to check, lessons learned the hard way. It is
  loaded only when that task comes up, so it can be long. Its description
  says when to use it. This file points to it.
- **`docs/`** holds what a person reads: how to build, how to deploy, how
  the API answers. Claude reads it too, when the task touches it.
- **Code comments** hold why a piece of code is the way it is.

A wish about how Tim works is a line here. A procedure is a skill. A fact
about the system is a doc. When in doubt, a rule here and the details in a
skill or a doc it names.
