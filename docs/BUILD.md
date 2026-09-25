# Building with make

One command, from the top of the repository:

```
make run
```

On a fresh Mac this installs what is missing, builds the app, starts a local
database, loads the starting sources and seeds, and opens the app on
http://localhost:8080. The first time, it writes `.env` with a login for this
machine and prints the password. It stays in `.env` as `LOCAL_PASSWORD`.

## What make does

1. **Installs what this machine is missing.** On macOS that is Homebrew doing
   Go, Node.js and PostgreSQL 16. Elsewhere it says what to install, because
   that needs administrator rights. It costs nothing when nothing is missing:
   every check is a `command -v` or a file test.
2. **Checks** for Go 1.27 or newer and Node.js 22 or newer, and stops with
   the install command if one is still missing.
3. **Installs the interface's packages** into `web/node_modules`, exactly as
   locked, when the lock file changed.
4. **Builds the interface** into `web/dist/`, when something in `web/`
   changed. Go embeds it, so the app is one file.
5. **Builds** `bin/speakertrail`, when a Go file, a migration or the
   starting data changed.

A second `make` with nothing changed takes a moment.

`make run` then also:

6. **Starts PostgreSQL** if it is not running, Homebrew's service on macOS,
   and creates the `speakertrail` role and database.
7. **Writes `.env`** the first time, with the database address, a login and
   a session secret.
8. **Loads the starting data** from the brief. It adds only what is missing,
   so it is safe every time.
9. **Starts the app.**

**A build runner never installs anything.** `CI` in the environment turns
step 1 off. `make INSTALL=0` does the same by hand.

## Targets

| Command | What it does |
| --- | --- |
| `make` | steps 1 to 5 |
| `make run` | everything above, then the app on http://localhost:8080 |
| `make crawl` | checks every due source once, like the nightly job. Uses Chrome or Chromium for pages that need JavaScript when it finds one |
| `make ui` | the interface with live reload on http://localhost:5173, sending the API to a `make run` in another terminal |
| `make mock` | the interface alone with invented data and no backend. The mock password is `speakertrail` |
| `make test` | `unit` and `interface` |
| `make unit` | every Go test under the race detector, against the local database. Each test makes its own throwaway database |
| `make interface` | a type check of the interface |
| `make check` | what this machine has and what it still needs, with the command for each |
| `make tools` | the installing part of `make` and nothing else |
| `make db` | starts the local database and sets it up |
| `make pull-prod` | copies the production database into the local one, see below |
| `make image` | builds the production Docker image, needs Docker |
| `make clean` | removes `bin/`, `web/node_modules/` and the built interface |
| `make help` | this list |

## Settings

`.env` holds the local settings. Anything set in the environment wins over
it. The file is not in the repository.

| Setting | What it is |
| --- | --- |
| `DATABASE_URL` | the database, local by default |
| `TEST_DATABASE_URL` | where tests make their throwaway databases |
| `LOCAL_PASSWORD` | the login for this machine, for you to read |
| `UI_PASSWORD_HASH` | its hash, which the app checks |
| `SESSION_SECRET` | signs the login cookie |
| `PORT` | where the app listens, 8080 |
| `CHROME_PATH` | a Chromium to use, when it is not found by itself |

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

## Other systems

On Linux, install Go 1.27 from https://go.dev/dl, Node.js 22 from
https://nodejs.org and PostgreSQL 16, then `make run`:

```
sudo apt install postgresql-16 chromium
```

`make` starts the system's PostgreSQL cluster when it is not running.

## Without make

```
(cd web && npm ci && npm run build)
go build -o bin/speakertrail ./cmd/speakertrail
bin/speakertrail import
bin/speakertrail serve
```

with `DATABASE_URL`, `UI_PASSWORD_HASH` and `SESSION_SECRET` set.
`bin/speakertrail hash-password` makes the hash.
