# Building with make

Everything runs in Docker. The one thing your Mac needs is
[Docker Desktop](https://www.docker.com/products/docker-desktop/). Go,
Node.js, PostgreSQL and Chromium live in containers, never on the Mac.

From the top of the repository:

```
make run
```

This builds the app, starts its database, loads the starting sources,
and starts the app on http://localhost:8080. Ctrl-C stops it. The
first time, it writes `.env` with a login for this machine and prints the
password. It stays in `.env` as `LOCAL_PASSWORD`.

The first run downloads the base images and takes a few minutes. After that
a rebuild takes seconds.

## What runs where

`compose.yaml` describes four containers:

| Container | What it is |
| --- | --- |
| `postgres` | the database, PostgreSQL 16 like on Scaleway. Its data stays in a Docker volume between runs |
| `web` | the app, built from the same `Dockerfile` that is deployed |
| `nightly` | the same image, doing one crawl |
| `dev` | the toolbox with Go, Node.js and Chromium, for tests and the live interface. The repository is mounted into it, its caches stay in Docker volumes |

Only the app, on port 8080, and the live interface, on port 5173, are
reachable from the Mac, and only from the Mac itself.

## Targets

| Command | What it does |
| --- | --- |
| `make run` | builds and starts the app with its database, on http://localhost:8080 |
| `make` | builds the app |
| `make crawl` | checks every due source once, like the nightly job |
| `make report` | writes `report.md`: the last runs, what failed and what waits, without anyone's name. Attach it to a chat with Claude when something looks wrong |
| `make people` | who is on stage on 5 event pages the runs found, by the engine's rules and by the local model, see below. `URL="…"` names the pages instead. Stores nothing |
| `make ui` | the interface with live reload on http://localhost:5173, sending the API to a `make run` in another terminal |
| `make mock` | the interface alone with invented data, on http://localhost:5173. The mock password is `speakertrail` |
| `make test` | `unit` and `interface` |
| `make unit` | every Go test under the race detector. Each test makes its own throwaway database |
| `make interface` | a type check of the interface |
| `make shell` | a shell in the toolbox |
| `make check` | whether Docker is installed and running |
| `make pull-prod` | copies the production database into the local one, see below |
| `make image` | builds the production image, as the deployment does |
| `make stop` | stops every container make started |
| `make clean` | removes the built app, the images and the caches. The database stays |
| `make help` | this list |

## The language model

The engine reads people better with a language model. Every run reads the
own pages of upcoming events with it, up to 10 per run, and stores who is
on stage with the passage that shows it. When the page says someone founded
or runs something, they count as a founder, and People shows founders
first. A title counts too, when its role says founder, Gründerin or owner:
"Projektleiter, Gründerzentrum" does not, and neither does a CEO alone.
`make report` says for each founder which passage or title made them one. It runs on this Mac,
not with a paid service: Docker Desktop's Model Runner runs it on the Mac's
graphics chip, and the app's container asks it.

`make run` and `make crawl` get the model when Model Runner is on. Without
it the app works the same and only reads no event pages. Model Runner is on
by default in Docker Desktop on Apple silicon. If make says it is off:

```
docker desktop enable model-runner
```

The first `make people` downloads the model, `ai/gemma3:12b-q4_K_M`, about
8 GB. After that it starts in seconds. To try another one, name it:

```
make people MODEL=ai/gemma3:4b-q4_K_M
```

Without `URL`, `make people` takes the pages of single upcoming events that
runs found, one per source. So start a run in the app first, on Runs.

The model answers one question at a time. When it fails, for example
because the Mac runs short of memory with two models loaded, the run leaves
the remaining event pages for a later run instead of retrying them, and
`make report` lists the failure.

Each line the model gives comes with the passage from the page that puts
the person on stage. A name the page does not contain is left out, and so is
anyone who is only a contact person or in the imprint.

## Settings

`.env` holds the login for this machine. It is not in the repository.

| Setting | What it is |
| --- | --- |
| `LOCAL_PASSWORD` | the password, for you to read |
| `UI_PASSWORD_HASH` | its hash, which the app checks |
| `SESSION_SECRET` | signs the login cookie |

To change the password, delete `.env` and run `make run` again.

## Production data on this machine

Data flows one way only, from production down. Code and database structure
travel through Git, as migrations that both sides apply on start.

```
PROD_DATABASE_URL='postgres://…' make pull-prod
```

The address is the GitHub secret `DATABASE_URL`. This replaces the local
database with a copy of production. The copy holds real names, so it stays
on this machine and never goes into the repository. Nothing ever goes back
up: changes meant for production are made in the production app.

## Removing it all

`make clean` keeps the local database. To remove the database as well:

```
make clean
docker volume rm speakertrail_postgres
```

## Without Docker

Build runners, Claude Code cloud sessions and the toolbox itself run the
same targets directly, with Go 1.27, Node.js 22 and PostgreSQL 16 already
there. `make DOCKER=0 …` does the same on any machine that has them.
`make DOCKER=0 check` says what is missing. Nothing is ever installed by
make or by the program.
