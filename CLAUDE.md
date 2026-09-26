# CLAUDE.md

Guidance for Claude when working on this repository. Read it fully before the
first change of a session. The user is Tim.

## The project

Speaker Trail finds in-person events across North Rhine-Westphalia and the
people who speak, pitch or host at them. A crawler checks a growing list of
sources every night and learns which ones are worth checking. The front is a
public event calendar, "Who's on stage in NRW", useful to many people. Behind
a single login is Tim's workspace: the numbers, the people, the sources and
the crawl runs.

It began as a way to find guests for Tim's podcast, *My First Memory*. The
direction since is a calendar for everyone, kept apart from Tim's LinkedIn,
which stays about the podcast.

## Where things are

Start with [README.md](README.md). In short:

| Part | Code | Docs |
| --- | --- | --- |
| fetching, robots.txt, the headless browser | `internal/fetch/` | |
| events and people out of pages | `internal/extract/` | |
| source checks, resolving, lifecycle, nightly run | `internal/pipeline/` | |
| job queue | `internal/queue/` | |
| HTTP API and login | `internal/server/` | [docs/api.md](docs/api.md) |
| interface, Svelte 5 | `web/` | `.claude/skills/interface/` |
| starting data from the brief | `internal/importer/` | |
| build, Docker | `Makefile`, `compose.yaml`, `Dockerfile`, `scripts/` | [docs/BUILD.md](docs/BUILD.md), `.claude/skills/docker/` |
| working with Claude | | [docs/WORKFLOW.md](docs/WORKFLOW.md) |
| deployment | `Dockerfile`, `deploy/`, `.github/` | [deploy/scaleway/README.md](deploy/scaleway/README.md) |

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
- His machine is an M2 Max with 32 GB of memory, on the latest macOS. He
  does not want apps and tools installed on it. Docker Desktop is the one
  exception, everything else runs in containers.
- He tests the interface himself on his phone and his Mac. After each change
  say exactly what to look at and what should happen.
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
  A **run** is one pass over the due sources, nightly or started by hand. A
  link to one event, added as a source, becomes the calendar it belongs to.
  Seeds exist only in the starting data, never in the interface. An
  **event** is kept or dropped, never accepted or rejected. A **profile** is
  confirmed or rejected. The public page is the **calendar**, Tim's side is
  the **workspace**. Whatever a thing is called in the interface, it is
  called that in the docs and the code comments too.
- Docs live in `docs/`. Only `README.md` and this file sit at the top, and
  the deployment guide sits beside what it deploys, in `deploy/`.
- When behaviour changes, update the matching doc in the same change.

## Interface rules

- **Phone first.** Tim reviews on his phone. Every screen works at 390 px
  wide with no sideways scrolling, and the first view of every screen
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
- **Nothing on Tim's Mac but Docker.** Every tool the project needs, Go,
  Node.js, Postgres, Chromium and whatever comes later, lives in a container
  from `compose.yaml` and the `Dockerfile`. A new tool goes into the `dev`
  stage of the `Dockerfile`, never into instructions for his Mac. Never tell
  him to `brew install` or `npm install -g` anything.
- **One command.** `make run` goes from a fresh clone to a running app.
  Nobody should have to remember a second command. On a laptop make drives
  Docker. Build runners, cloud sessions and the toolbox run the same targets
  directly, see `DOCKER` in the `Makefile`. Neither make nor the program
  ever installs anything. A new target works both ways, and CI's `toolbox`
  job proves the Docker way.
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
- **Short transactions.** No job holds a transaction open for more than a
  moment, because long ones break the database's automatic backups.
- `gofmt` and `go vet` must pass.
- The interface is built by make into `web/dist/`, which Go embeds. Only
  `web/dist/.keep` is in the repository. Never commit build output or
  `.env`.
- **The language model runs locally.** It runs on Tim's Mac through Docker
  Model Runner, with no paid model API, no key and no credit card. The
  engine works without it and only reads people better with it.
- Every paid API gets a monthly budget in Settings, and the engine stops
  calling it once the budget is reached.

## Cloud sessions

Sessions at claude.ai/code run on Ubuntu 24.04 with the setup script from
`scripts/cloud-setup.sh`, which provides Go 1.27 and a local Postgres 16.
`scripts/session-start.sh` starts Postgres and sets `DATABASE_URL` and
`TEST_DATABASE_URL` in every session.

- If `go version` does not show 1.27, run `bash scripts/cloud-setup.sh`.
- `make test` and `make` before pushing anything that touches Go. They run
  directly here, because `CLAUDE_CODE_REMOTE` is set.
- Docker works in a session, with limits. Before changing the Docker side,
  read the skill in `.claude/skills/docker/`.
- The network is limited to an allowlist set in the environment. Event sites
  that are not on it cannot be fetched from a session, and the deployed app
  has no such limit.
- There is no screen. Interface work goes through the skill in
  `.claude/skills/interface/`. Read it before changing anything in `web/`.

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
