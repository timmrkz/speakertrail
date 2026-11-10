# Building with make

Speaker Trail is a Mac app, built the way FrameFairy is: Go with Wails v3
for the window, the Svelte interface built into it, and everything it knows
in one plain file. No Docker, no database, no server.

From the top of the repository:

```
make run
```

The first time, make installs what the Mac is missing with Homebrew, Go
and Node.js, and the command line tools for the compiler. Then it builds
the interface and the app, puts them together as `bin/Speaker Trail.app`
and opens it. The log stays in the terminal. After the first time a
rebuild takes seconds.

To start it like any other app instead, from the Dock or Spotlight:

```
make install
```

That copies the app to `/Applications`. Run it again after `git pull` to
put the new version there.

## The first start

The app opens on People. Before the first run, paste the Exa key into
Settings. It goes into the macOS keychain, as **Speaker Trail: Exa API
key**, and never into a file. `EXA_API_KEY` in the environment stands in
while no key is saved.

## Where things are kept

| What | Where |
| --- | --- |
| People, searches, runs, settings | `~/Library/Application Support/Speaker Trail/speakertrail.json` |
| The Exa key | the keychain, as **Speaker Trail: Exa API key** |

The file is written whole on every change, to a new file that then takes
the old one's place, so a crash in the middle leaves the last good file.
It holds real names, so it stays on this Mac and never goes into the
repository. `SPEAKERTRAIL_DIR` names another folder for the file, for a
try that should not touch the real one.

## The people kept in the Docker version

The Docker version kept its database in a Docker volume. Once, while
Docker Desktop still runs and with the app closed:

```
make carry-over
```

This reads the people you kept out of the old database, with Postgres 16
in a container, and adds them to the app as kept, with their LinkedIn
profile and notes. It changes nothing in the old database and is safe to
repeat. Afterwards Docker is no longer needed, and the old data can go:

```
docker volume rm speakertrail_postgres
```

## Targets

| Command | What it does |
| --- | --- |
| `make run` | installs what is missing, builds the app and opens it |
| `make` | builds the app, `bin/Speaker Trail.app` on a Mac |
| `make install` | the same, copied to `/Applications` |
| `make carry-over` | the people kept in the Docker version, into the app, see above |
| `make mock` | the interface alone in a browser with invented data, on http://localhost:5173 |
| `make test` | `unit` and `interface` |
| `make unit` | every Go test under the race detector |
| `make interface` | a type check of the interface |
| `make check` | what this machine has and still needs |
| `make clean` | removes what make built. The app's file stays |
| `make help` | this list |

## Elsewhere than on a Mac

Build runners and Claude Code cloud sessions run the same targets. There
make installs nothing, and `CI` in the environment turns installing off
everywhere. On Linux the app's window compiles against GTK 4 and
WebKitGTK 6, `libgtk-4-dev` and `libwebkitgtk-6.0-dev`, which
`scripts/cloud-setup.sh` installs in a cloud session. `make` there builds
`bin/speakertrail-app` and the tests run, but no window can be opened.
