# Speaker Trail

Speaker Trail finds in-person events across North Rhine-Westphalia and the people who speak, pitch or host at them. It turns them into a twice-weekly LinkedIn post and outreach picks for Tim.

It is one Go binary with three commands.

| Command | What it does |
| --- | --- |
| `speakertrail migrate` | Applies the database migrations |
| `speakertrail serve` | Runs the web interface and its API. For now only `/healthz` |
| `speakertrail worker` | Runs the job queue. `--until-idle` exits once no job is due, for the nightly scheduled run |

## Layout

| Path | Contents |
| --- | --- |
| `cmd/speakertrail` | The command |
| `internal/config` | Secrets and deployment settings from environment variables |
| `internal/db` | Database pool and the embedded migrations in `internal/db/migrations` |
| `internal/queue` | Job queue, worker and the per-website rate limit |
| `internal/dbtest` | A fresh, migrated database for each test |
| `scripts` | The cloud environment setup script and the session start hook |

## Job queue

- Kind plus key is unique. Enqueueing work that is already queued or running does nothing, so every run can be repeated safely. Finished or failed work with the same key is queued again from scratch.
- Workers claim jobs with `FOR UPDATE SKIP LOCKED` and hold them by a 10 minute lease, not by an open transaction. A job whose worker crashed is taken over once the lease runs out.
- A failed job is retried after 1 minute, 10 minutes and 1 hour, then every hour. After 5 attempts it stays failed with its last error.
- Every website gets at most 1 request every 5 seconds, across all workers. The fetcher reserves a slot in the `site_slots` table and waits for it.

## Environment variables

| Variable | Needed by |
| --- | --- |
| `DATABASE_URL` | All commands |
| `TAVILY_API_KEY` | Discovery and profile lookups, from batch 7 |
| `ANTHROPIC_API_KEY` | Post drafts, from batch 9 |
| `UI_PASSWORD_HASH` | The login, from batch 5 |
| `SESSION_SECRET` | The login, from batch 5 |
| `PORT` | `serve`, default 8080 |
| `TEST_DATABASE_URL` | Tests. The role must be allowed to create databases |

## Local development

You need Go 1.27 and Docker.

1. Start Postgres 16.

   ```sh
   docker compose up -d postgres
   ```

2. Point the tools at it.

   ```sh
   export DATABASE_URL="postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable"
   export TEST_DATABASE_URL="$DATABASE_URL"
   ```

3. Apply the migrations and start a worker.

   ```sh
   go run ./cmd/speakertrail migrate
   go run ./cmd/speakertrail worker
   ```

4. Run the tests. Each test creates its own database and drops it afterwards.

   ```sh
   go test -race ./...
   ```

Without `TEST_DATABASE_URL` the database tests are skipped locally. In CI they fail instead.

## Claude Code cloud sessions

The cloud environment runs `scripts/cloud-setup.sh` as its setup script. Paste its content into the environment's "Setup script" field. It installs Go and creates the local database role. At the start of each session `scripts/session-start.sh` starts Postgres and sets `DATABASE_URL` and `TEST_DATABASE_URL`.

## Continuous integration

GitHub Actions runs `gofmt`, `go vet` and the tests against Postgres 16 on every pull request and every push to `main`.
