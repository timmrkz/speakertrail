# Speaker Trail

Speaker Trail finds in-person events across North Rhine-Westphalia and the people who speak, pitch or host at them. Every night it checks a growing list of sources, keeps what fits and learns which sources are worth checking. The front is an event calendar anyone can use. Behind a login are the numbers, the people, the sources and the crawl runs.

It is one Go binary with the web interface built in.

| Command | What it does |
| --- | --- |
| `speakertrail serve` | Runs the web interface and its API, plus a small worker so "Check now" and pasted seeds run right away |
| `speakertrail nightly` | Checks every due source once, then exits. This is the scheduled job |
| `speakertrail worker` | Works on queued jobs until stopped |
| `speakertrail migrate` | Applies database migrations. Every other command does this too |
| `speakertrail import` | Loads the sources and seeds from the brief. Safe to repeat |
| `speakertrail fetch <url>` | Shows what the engine finds on one page. `-browser` loads it with JavaScript, `-save file` keeps the page |
| `speakertrail hash-password` | Prints the hash for `UI_PASSWORD_HASH` |

## How it works

1. **Schedule.** Each night the due sources are queued: active and probation sources, retired ones whose recheck is due, and up to 10 new candidates.
2. **Fetch.** Plain HTTP first, with an honest User-Agent and robots.txt respected, at most 1 request every 5 seconds per website. When a page is an empty JavaScript shell, the headless browser loads it instead, and the source remembers that. LinkedIn and Instagram are never requested, not even by the browser.
3. **Extract.** iCal feeds, schema.org Event data, then adapters for Luma, Meetup and Eventbrite. Names come only from clearly marked lines like "Speaker: ..." or "Jury: ...".
4. **Resolve.** Events, people and organisations are merged with what is known. Every one of them remembers which source showed it.
5. **Fit.** Online events, events outside the region and titles with a drop word are dropped. Everything else in NRW is kept. Your own keep or drop always wins.
6. **Learn.** Linked Meetup, Luma and Eventbrite calendars become new candidate sources. Sources move from candidate to probation to active, and retire when they stop producing.

The source list, the fit rules and every number live in the Settings screen.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/speakertrail` | The command |
| `internal/fetch` | HTTP, robots.txt, the headless browser and the filter that keeps it away from blocked sites |
| `internal/extract` | Turns pages into events and people |
| `internal/pipeline` | Source checks, resolving, the source lifecycle, the nightly run, seeds |
| `internal/server` | The JSON API and the login, described in `docs/api.md` |
| `internal/queue` | Job queue in Postgres with retries and the per-website limit |
| `internal/importer` | The starting data from the brief |
| `internal/db` | Migrations |
| `web` | The interface, Svelte 5 with TypeScript |
| `deploy` | Scaleway and plan B |

## Running it locally

You need Go 1.27, Node 22 and Docker.

```sh
docker compose up -d postgres
export DATABASE_URL="postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable"
export UI_PASSWORD_HASH="$(go run ./cmd/speakertrail hash-password)"
export SESSION_SECRET="$(openssl rand -hex 32)"

(cd web && npm ci && npm run build)
go run ./cmd/speakertrail import
go run ./cmd/speakertrail serve
```

Open http://localhost:8080 and log in. To crawl once, run `go run ./cmd/speakertrail nightly`. It uses Chromium when it finds one, or the one named in `CHROME_PATH`.

To work on the interface with live reload, run `npm run dev` in `web/` next to `serve`. `MOCK=1 npm run dev` runs it with example data and no backend.

The whole system also runs in Docker: `docker compose up -d postgres web`, and `docker compose run --rm nightly` for a crawl.

## Tests

```sh
export TEST_DATABASE_URL="$DATABASE_URL"
go test ./...
(cd web && npm run check)
```

Each database test gets its own throwaway database. Without `TEST_DATABASE_URL` those tests are skipped locally and fail in CI. Tests never touch a live website. The pages in `internal/extract/testdata` are synthetic, with invented people. One test runs the real browser through a recording proxy to prove it never reaches LinkedIn or Instagram.

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
