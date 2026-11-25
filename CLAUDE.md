# CLAUDE.md

Guidance for Claude when working on this repository. Read it fully before the
first change of a session. The user is Tim.

## The project

Speaker Trail finds people in North Rhine-Westphalia who run their own
business and could be guests on Tim's podcast, *My First Memory*. It is a
Mac app, built like Tim's FrameFairy: Go with Wails v3 for the window, the
Svelte interface built in, and everything it knows in one JSON file in
`~/Library/Application Support/Speaker Trail`, with the profile texts in
a second beside it. Runs ask profile searches
through Exa. No Docker, no database, no server.

It once also crawled event pages for a public calendar. That engine, its
Postgres database, Docker and the Scaleway deploy were removed on 10
November 2026 and are in git history.

## Where things are

Start with [README.md](README.md). In short:

| Part | Code | Docs |
| --- | --- | --- |
| the app, its window | `cmd/speakertrail-app/` | |
| the file: people, searches, runs, settings | `internal/store/` | |
| runs of profile searches | `internal/engine/` | |
| HTTP API the interface talks to | `internal/api/` | [docs/api.md](docs/api.md) |
| the Exa key in the keychain | `internal/keys/` | |
| the app's log, rotated, in `~/Library/Logs/Speaker Trail` | `cmd/speakertrail-app/applog.go` | [docs/LOGGING.md](docs/LOGGING.md) |
| Report a Problem, its screen recordings, Dictate, the speech models in Settings | `cmd/speakertrail-app/bugs.go`, `report.go`, `recording.go`, `listen.go`, `speechmodels.go`, `internal/speech/` | [docs/LOGGING.md](docs/LOGGING.md), [docs/api.md](docs/api.md) |
| Draft, the first message to a person, its template, the language model and llama-server | `internal/draft/`, `internal/store/template.go`, `internal/language/`, `cmd/speakertrail-app/languagemodels.go`, `scripts/build-llama.sh` | [docs/api.md](docs/api.md), [docs/BUILD.md](docs/BUILD.md) |
| updates: builds per pull request, signing, the Updates page | `internal/updates/`, `cmd/speakertrail-release/`, `.github/workflows/builds.yml`, `publish.yml` | [docs/UPDATES.md](docs/UPDATES.md) |
| Exa, whether a place is in NRW | `internal/search/`, `internal/place/` | |
| opening LinkedIn in Tim's Chrome profile | `internal/browser/` | [docs/open-in-profile.md](docs/open-in-profile.md) |
| interface, Svelte 5 | `web/` | [docs/INTERFACE.md](docs/INTERFACE.md) |
| build, the .app bundle | `Makefile`, `scripts/` | [docs/BUILD.md](docs/BUILD.md) |
| who to look for, and where, the fit rubric | `internal/rubric/` | [docs/search-strategy.md](docs/search-strategy.md), [docs/fit-score.md](docs/fit-score.md) |
| working with Claude, telling a pull request that conflicts with main | `.github/workflows/conflicts.yml`, `cmd/speakertrail-release/conflicts.go` | [docs/WORKFLOW.md](docs/WORKFLOW.md) |

## The skills

How we work is in the skills in `.claude/skills/`. Every one is written
for any project, so it names nothing of ours. Read the one for the work before starting it. Where a
skill and this file differ, this file wins. Most come from FrameFairy as
they are, see [docs/SKILLS.md](docs/SKILLS.md), which also says where
each skill's words meet this repository: the user is Tim, a review goes
in `/mnt/project-files/reviews/`, and so on.

| Skill | For |
| --- | --- |
| [engineering](.claude/skills/engineering/SKILL.md) | the first principles of any change, and which skill holds which part |
| [architecture](.claude/skills/architecture/SKILL.md) | where a change goes, the server and client split, changing the design |
| [maintenance](.claude/skills/maintenance/SKILL.md) | evidence before work, root causes, problems that come back, issues and triage |
| [testing](.claude/skills/testing/SKILL.md) | how every test is written, fixed and reviewed, and how a bug is found |
| [ci](.claude/skills/ci/SKILL.md) | which jobs a change starts, caches, limits, measuring CI |
| [security](.claude/skills/security/SKILL.md) | untrusted input, paths, programs, secrets, signing, the supply chain, workflows |
| [pull-requests](.claude/skills/pull-requests/SKILL.md) | how a pull request is opened, sized, described, watched, made ready and merged |
| [review](.claude/skills/review/SKILL.md) | how a pull request is reviewed by an agent that did none of the work |
| [interface-design](.claude/skills/interface-design/SKILL.md) | what goes on screen, how actions are answered, one look, layout |
| [interface](.claude/skills/interface/SKILL.md) | how an interface nobody can see is changed and proven, see [docs/INTERFACE.md](docs/INTERFACE.md) for ours |
| [collaboration](.claude/skills/collaboration/SKILL.md) | small batches, replies, the repository instead of memory, priority and size, writing |

`.claude/settings.json` raises the room Claude Code gives the list of
skills, `SLASH_COMMAND_TOOL_CHAR_BUDGET`, to 80000 characters. Below
that, the skills listed last show their name without what they are for.

## How Tim works

- He uses Claude through claude.ai, in chat and in cloud sessions at
  claude.ai/code. Changes reach him as pull requests on GitHub. He tries
  one by picking it on the app's Updates page, which downloads its build
  and relaunches, see [docs/UPDATES.md](docs/UPDATES.md). No terminal, no
  `git pull`. Never hand him zip files or ask him to copy files around.
- **Every piece of work becomes a pull request, always.** A branch he has to
  find himself is work he cannot see.
- **The pull request comes first, not last.** Push the first commit and open
  the pull request straight away, without being asked, and keep pushing to
  it. The first commit does not have to be worth looking at. It exists so
  the pull request exists, because that is where Tim follows the work.
- His machine is an M2 Max with 32 GB of memory, on the latest macOS. What
  the build needs, Go, Node.js and CMake for llama-server, make installs
  with Homebrew, as it does for FrameFairy. Nothing beyond that, and never as a step he has to do.
- He tests the app himself on his Mac. After each change say exactly what
  to look at and what should happen.
- He answers in English or German. Reply in the language of his message.
  He often dictates, so FrameFairy can arrive as "Frame variable" or
  "FrameFerry".
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
- **A port copies FrameFairy as it is.** Change only what Speaker Trail
  makes necessary and name every difference, never redesign it (Tim, 13
  and 21 November 2026). Copy from FrameFairy's main as it stands that
  day. Tim's standing exceptions: requests over HTTP `/api` instead of
  Wails bindings (14 November), the flat look (#45), and updates that
  download only what changed (#40).
- **Report plainly.** What changed, what to test and how, what is still open.
  Name bugs you found on the way, including your own.
- **Watch a pull request, quietly.** Subscribe to its events and act on
  them: a failing CI run is still your work, so is a review comment. But
  never set up a recurring check, never poll, and never write a message that
  says nothing happened. Tim comes back when he is ready. But a CI
  result, green or red, is news, and what an event starts, tell Tim in
  the thread, and again when it is done.
- **Merge main in for a conflict, and only then.** When a pull request no
  longer merges into main, merge main in, resolve it and push at once,
  without waiting for an answer. Tell Tim in one line which files
  conflict, and again once it is pushed, since a checklist notifies no
  one. `conflicts.yml` comments on such a pull request, which
  wakes its session, and a session checks itself right after opening one
  and before calling it ready, with git, since GitHub may not know yet.
  The resolution is new code: run the tests of what it touches, then
  push and let CI run. Read this file again after the merge, because main
  may have changed it. See When main moves in
  [docs/WORKFLOW.md](docs/WORKFLOW.md).

## Writing rules

These apply to everything written for Tim: interface text, docs, commit
messages, pull request text, code comments and chat replies.

- **Never use semicolons.** Use commas and full stops. Semicolons in code
  syntax are fine, in prose and comments they are not.
- Plain, direct language. Short sentences.
- **One name per thing.** A **profile search**, or **search** for short,
  asks Exa for LinkedIn profiles, like "Yogalehrerin Köln", and keeps the
  people in NRW who were active lately and run their own business. An
  **exclusion** is a word or phrase, like "Online Marketing", that leaves a
  person out of profile searches and People. A **first message** is what
  Tim sends a person on LinkedIn once connected, and a **draft** is one
  the app put together on the Mac for him to change: one **variation** of
  each **part** of the **template** in Settings, picked at random, and the
  **hook**, the portrait, reason and reaction the **language model**
  wrote. A **run** is
  one pass over the due searches and the profiles of links Tim added,
  started by hand or by adding links. A **check** is one search asked once in a
  run. A **person** is connected or not, never kept or skipped: Tim marks
  whom he connected with on LinkedIn, and the rest is simply open. A
  **profile** is confirmed or rejected. A **channel** is what the app
  updates from, main or one pull request, and a **build** is the app made
  from one commit of it. The API still says `sources` for
  searches, the interface and the docs never do. Whatever a thing is
  called in the interface, it is called that in the docs and the code
  comments too.
- Docs live in `docs/`. Only `README.md` and this file sit at the top.
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
- **A control is as wide as what it shows now.** A button fits the words
  it says, Check while it says Check and Checking while it checks, as in
  FrameFairy. A list to pick from fits what is picked, up to
  `--pick-max`. Never a width kept for other words or guessed in `em`
  (Tim, testing #46 on 21 November 2026).
- **Controls in one row have the same size.** No one-off sizes.
- **Two gaps between controls.** `--gap-control` between two controls,
  `--gap-inset` between items on one ground, like the segments of a
  choice and the pages in the sidebar (Tim, 21 November 2026, #63).
- **What cannot be taken back asks first.** Everything else saves at once
  and is undone with a click. No save buttons, except a form that adds
  something new.
- **A click shows at once.** A control that starts work says what it is
  doing and takes no second click.
- **Work never runs silently.** Anything that takes more than a moment
  says what it does now, how far it is, how long it has run and about how
  long is left, measured from earlier work rather than guessed, and it can
  be stopped. It looks the same everywhere: the beam and the fill of
  `Busy.svelte` on the control that started it and on the row of what
  it works on, and the pulsing dot in `app.css`. No line of progress
  under a toolbar (Tim, 22 November 2026, #43). See
  [docs/INTERFACE.md](docs/INTERFACE.md#work-in-hand).
- **Short runs, often.** Tim runs the engine by hand on his Mac. A run
  stays short enough to watch, and what does not fit waits for the next.
- **The sidebar is FrameFairy's.** A rail of icons until the pointer
  reaches it, open over the page while it is there, kept open by the
  button at its top. People and Searches at the top, Settings, Bugs and
  then Updates at the foot. `lib/sidebar.svelte.ts` and its walks in
  `web/walks/sidebar.mjs` are ported from FrameFairy, so a fix there is a
  fix here.
- **Consistency over novelty.** A visual treatment used in one place is used
  for every equivalent element, or not at all. Two things of the same kind
  are the same size, in the same colour, in the same place.
- Colours and sizes come from the tokens in `web/src/app.css`, in both
  themes. Tabular figures for times and counts.
- **Flat.** No shadows, no blur, no glassy edges, solid colours in both
  themes (Tim, 21 November 2026). Hover, pressed, chosen and greyed out
  come from the state tokens in `app.css`, the same for every control.
- **Quiet marks.** No lines or patterns in the background. A button in
  focus wears a ring in its own colour, a field in use keeps the grey
  edge it has under the pointer, never the accent (Tim, 21 November
  2026). See [docs/INTERFACE.md](docs/INTERFACE.md#the-look).

## Engineering rules

- **Go, latest version.** One module, `go 1.27` in `go.mod`. Upgrade with Go
  releases.
- **Built like FrameFairy.** A native Mac app with Wails v3, no Docker, no
  database process, no server. What the app knows lives in one JSON file,
  written whole to a new file that then replaces the old one. Only the
  profile texts live in a second, `profile-texts.json`, written only when
  one arrives, because they made every save ten times slower (Tim, 20
  November 2026). Secrets live
  in the macOS keychain, never in a file.
- **One command.** `make run` goes from a fresh clone to a running app.
  Nobody should have to remember a second command. On a Mac make installs
  what is missing with Homebrew, `scripts/tools.sh`, and nothing else ever
  installs anything. Never tell Tim to `brew install` or `npm install -g`
  anything himself. With `CI` set make installs nothing.
- **No shell scripts.** Tim does not want any: logic in shell nobody can
  read becomes a mess. What a build or a workflow has to work out is Go,
  in a program like `cmd/speakertrail-release`, with tests. A workflow
  step only calls tools that are there, like `gh` and `go run`. The
  scripts still in `scripts/` are from before this rule.
- **It builds on Linux too.** Build runners and cloud sessions build the
  window against GTK 4 and WebKitGTK 6 and run every test there. CI's
  `macos` job proves the Mac build, the bundle and the keychain.
- **No LinkedIn, no Instagram.** The app never requests them. Profiles
  come from Exa's index, asked with `livecrawl` never, by link and then
  by the name in it. One Exa lacks Tim copies from Chrome and pastes on
  the person (23 November 2026, #43). Never add a request to either.
- **LinkedIn opens in Tim's Chrome profile.** The app never requests
  LinkedIn itself, it hands the link to Chrome with
  `open -n -a "Google Chrome" --args --profile-directory=<folder>`, the
  profile where Tim is logged in. Never plain `open <link>`, which lands in
  whichever profile was used last. See [docs/open-in-profile.md](docs/open-in-profile.md).
- **Personal data.** Names, roles and profile links are personal data.
  Store only what the data model lists, never email addresses or phone
  numbers. The app's file stays on Tim's Mac. People leave it only in
  a report with People switched on, without their names or links, see
  `store.Shared` (Tim, 22 November 2026, #73). Real names never go into
  test fixtures, mock data or anything committed: invent them.
  The app's log never names a person either: whatever shows a person's
  words is marked `data-personal`, see [docs/LOGGING.md](docs/LOGGING.md).
- **Untrusted input.** What Exa answers is untrusted. The interface shows
  it as text, never as HTML.
- **What runs on its own goroutine is tested from several at once.** Every
  Go test runs under the race detector, and anything asynchronous, a run
  above all, has a test that uses it from several goroutines at once.
  A fix starts as a test that fails on the code before it.
- **Text runs nothing in CI.** A change only to Markdown, the docs and
  the skills, starts no job, and no test reads them for their words.
  Tim, 23 November 2026: updating Markdown files should not run
  anything, unless a check is very fast and very important.
- **Tests** need no network and never call Exa. Answers go in the test,
  with invented people. Run `make test` before every push that
  touches Go. A change only to `web/` or `docs/` runs `make interface`
  and `make walks`.
- **Short updates.** The file is written on every change, so nothing
  slow, above all no call to Exa, happens inside `Store.Update`.
- `make lint` must pass: `gofmt`, `go vet` and `.golangci.yml`. Our code
  never writes to Go's standard `log`, see [docs/LOGGING.md](docs/LOGGING.md).
- The interface is built by make into `web/dist/`, which Go embeds. Only
  `web/dist/.keep` is in the repository. Never commit build output, `.env`
  or the app's file.
- **No paid language model.** Whatever reads text needs no paid model
  API, no key and no credit card.
- Every paid API gets a monthly budget in Settings, and the engine stops
  calling it once the budget is reached.

## Cloud sessions

Sessions at claude.ai/code run on Ubuntu 24.04 with the setup script from
`scripts/cloud-setup.sh`, which provides Go 1.27, and GTK 4 and WebKitGTK 6
for the window.

- If `go version` does not show 1.27, or `pkg-config --exists gtk4
  webkitgtk-6.0` fails, run `bash scripts/cloud-setup.sh`.
- `make test` and `make` before pushing anything that touches Go.
- The network is limited to an allowlist set in the environment. Exa may
  not be on it, and tests never call it anyway.
- There is no screen, and the window cannot open. Interface work goes
  through the interface skill and [docs/INTERFACE.md](docs/INTERFACE.md),
  which has the harness. Read both before changing anything in `web/`.
- Hugging Face is not on the allowlist, so no real speech or language
  model runs in a session. Tim tries those on his Mac.
- A real build is read from the `dev` release, whose assets are public,
  with Go's `debug/macho`.
- A `macos` job cancelled at 15 minutes with `runner_id` 0 never got a
  runner. That is not a broken build. Run it once more.
- A session's branch is fixed, and force-push is refused. After its pull
  request is squash-merged, or for a second pull request, ask for a new
  thread with its own branch.

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
- **Open work** is a GitHub issue.
- **Never the project memory at claude.ai.** Nobody reviews it and no
  pull request changes it, so it drifts. It holds only a pointer to this
  repository (Tim, 21 November 2026).

A lesson about how to work goes into a skill, in words any project can
use. A rule only Speaker Trail needs is a line here. A procedure is a
skill. A fact about the system is a doc. When in doubt, a rule here and
the details in a skill or a doc it names.
