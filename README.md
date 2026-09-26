# Speaker Trail

Speaker Trail finds in-person events across North Rhine-Westphalia and the people who speak, pitch or host at them. Every night it checks a growing list of sources, keeps what fits and learns which sources are worth checking. The front is an event calendar anyone can use. Behind a login are the numbers, the people, the sources and the crawl runs.

It is one Go binary with the web interface built in.

| Command | What it does |
| --- | --- |
| `speakertrail serve` | Runs the web interface and its API, plus a worker, so "Check now" and "Start a run" work right away |
| `speakertrail nightly` | Checks every due source once, then exits. This is the scheduled job |
| `speakertrail worker` | Works on queued jobs until stopped |
| `speakertrail migrate` | Applies database migrations. Every other command does this too |
| `speakertrail import` | Loads the starting sources from the brief. Safe to repeat |
| `speakertrail fetch <url>` | Shows what the engine finds on one page. `-browser` loads it with JavaScript, `-save file` keeps the page |
| `speakertrail people <url>...` | Who is on stage on event pages, by the rules and by the local language model. `-file` reads saved pages |
| `speakertrail hash-password` | Prints the hash for `UI_PASSWORD_HASH` |

## How it works

1. **Schedule.** Each night the due sources are queued: active and probation sources, retired ones whose recheck is due, and up to 10 new candidates.
2. **Fetch.** Plain HTTP first, with an honest User-Agent and robots.txt respected, at most 1 request every 5 seconds per website. When a page is an empty JavaScript shell, the headless browser loads it instead, and the source remembers that. LinkedIn and Instagram are never requested, not even by the browser.
3. **Extract.** iCal feeds, schema.org Event data, then adapters for Luma, Meetup and Eventbrite. Names come only from clearly marked lines like "Speaker: ..." or "Jury: ...".
4. **Read.** With the local language model, a run also reads each upcoming event's own page and finds who is on stage, with the passage that shows it, and whether the page says they founded or run something. Without the model this step is left out.
5. **Resolve.** Events, people and organisations are merged with what is known. Every one of them remembers which source showed it.
6. **Fit.** Online events, events outside the region and titles with a drop word are dropped. Everything else in NRW is kept. Your own keep or drop always wins.
7. **Learn.** Linked Meetup, Luma and Eventbrite calendars become new candidate sources. Sources move from candidate to probation to active, and retire when they stop producing.

The source list, the fit rules and every number live in the Settings screen.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/speakertrail` | The command |
| `internal/fetch` | HTTP, robots.txt, the headless browser and the filter that keeps it away from blocked sites |
| `internal/extract` | Turns pages into events and people |
| `internal/pipeline` | Source checks, resolving, the source lifecycle, the nightly run |
| `internal/server` | The JSON API and the login, described in `docs/api.md` |
| `internal/queue` | Job queue in Postgres with retries and the per-website limit |
| `internal/importer` | The starting data from the brief |
| `internal/db` | Migrations |
| `web` | The interface, Svelte 5 with TypeScript |
| `deploy` | Scaleway and plan B |
| `docs` | the API, building, and working with Claude |

## Running it locally

Everything runs in Docker, so the only thing to install is
[Docker Desktop](https://www.docker.com/products/docker-desktop/). Then:

```sh
make run
```

It builds the app, starts a local database, loads the starting sources and
starts the app on http://localhost:8080. The first time it prints a password
for this machine, which stays in `.env`. `make help` lists everything else,
and [docs/BUILD.md](docs/BUILD.md) explains it.

`make crawl` checks every due source once, like the nightly job, and `make
test` runs all tests.

## Environment variables

| Variable | Needed by |
| --- | --- |
| `DATABASE_URL` | Everything |
| `UI_PASSWORD_HASH` | The login |
| `SESSION_SECRET` | The login, at least 16 characters |
| `PORT` | `serve`, default 8080 |
| `CHROME_PATH` | Optional, the Chromium to use |
| `TAVILY_API_KEY` | Search and profile lookups, later |
| `ANTHROPIC_API_KEY` | Language model features, later |
| `TEST_DATABASE_URL` | Tests |

## Deployment

Every merge to `main` deploys to Scaleway once CI is green. The one-time setup is in `deploy/scaleway/README.md`. On a plain server, `compose.yaml` and `deploy/systemd` run the same system.

## Claude Code cloud sessions

The cloud environment runs `scripts/cloud-setup.sh` as its setup script. At the start of each session `scripts/session-start.sh` starts Postgres and sets `DATABASE_URL` and `TEST_DATABASE_URL`.

## Rules the code keeps

- No request ever goes to linkedin.com or instagram.com. Profile links only come from event pages and, later, search results.
- robots.txt is respected. A site that refuses the bot becomes a Manual source.
- No email addresses or phone numbers are stored. People are shown to the public only when the `public_show_people` setting is on.
- Stored pages are deleted after 30 days.
