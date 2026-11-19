# Building with make

Speaker Trail is a Mac app, built the way FrameFairy is: Go with Wails v3
for the window, the Svelte interface built into it, and everything it knows
in one plain file. No Docker, no database, no server.

From the top of the repository:

```
make run
```

The first time, make installs what the Mac is missing with Homebrew, Go,
Node.js and CMake, and the command line tools for the compiler. Then it
builds llama-server, which takes a few minutes, see
[The language model](#the-language-model), the interface and the app,
puts them together as `bin/Speaker Trail.app` and opens it. The app's log
is in `~/Library/Logs/Speaker Trail`, see [LOGGING.md](LOGGING.md). After
the first time a rebuild takes seconds.

To start it like any other app instead, from the Dock or Spotlight:

```
make install
```

That copies the app to `/Applications`. Once is enough: from then on the
app updates itself from its Updates page, to main or to any open pull
request. [UPDATES.md](UPDATES.md) says how, and how to install it the
first time.

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
| The channel the app follows | `~/Library/Application Support/Speaker Trail/updates.json` |
| Builds downloaded to update to | `~/Library/Caches/Speaker Trail/builds` |
| The speech models Dictate hears with, and the language model that drafts first messages | `~/Library/Application Support/Speaker Trail/models` |
| llama-server, built once by make | `.build/llama` in the repository, never committed |
| The draft of a report | `~/Library/Application Support/Speaker Trail/report` |

The file is written whole on every change, to a new file that then takes
the old one's place, so a crash in the middle leaves the last good file.
It holds real names, so it stays on this Mac and never goes into the
repository. `SPEAKERTRAIL_DIR` names another folder for the file, for a
try that should not touch the real one.

## The people connected in the Docker version

The Docker version kept its database in a Docker volume. Once, while
Docker Desktop still runs and with the app closed:

```
make carry-over
```

This reads the people you connected with, or kept before that, out of the
old database, with Postgres 16 in a container, and adds them to the app as
connected, with their LinkedIn
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
| `make llama` | builds llama-server, when it is not built or `scripts/build-llama.sh` changed. `make` does it by itself on a Mac |
| `make install` | the same, copied to `/Applications`, where it updates itself |
| `make update-key` | makes the key builds are signed with, once, see [UPDATES.md](UPDATES.md) |
| `make carry-over` | the people connected in the Docker version, into the app, see above |
| `make mock` | the interface alone in a browser with invented data, on http://localhost:5173 |
| `make test` | `lint`, `unit`, `interface` and `walks` |
| `make lint` | `gofmt`, `go vet` and golangci-lint with the rules in `.golangci.yml` |
| `make unit` | every Go test under the race detector, and the test of `scripts/needs-build.sh` |
| `make interface` | a type check of the interface and its tests, with vitest |
| `make walks` | the interface driven in Chromium with invented data, the walks in `web/walks/` |
| `make check` | what this machine has and still needs |
| `make clean` | removes what make built. The app's file stays, and so does llama-server in `.build/llama`, which takes minutes to build again |
| `make help` | this list |

## CI

`.github/workflows/ci.yml` runs the targets above in three jobs. A pull
request starts only the jobs whose code it changed, by the filters of the
job `changes`, which reads the files GitHub lists for the pull request
with `dorny/paths-filter`. As in FrameFairy.

| Changed | Starts |
| --- | --- |
| Go: any file outside `web/`, the docs and `.claude/`, a script, the Makefile and the workflows among them, and the files of `web/` Go reads, its Go files and `web/dist/.keep`, the filter `goread` | `go`, `make lint` and `make unit`, and `macos`, the app built on a Mac and its Go tests |
| the interface: `web/`, the Makefile and `.github/workflows/` | `web`, `make interface` and `make walks`, and `macos` |
| only the docs, `docs/`, any `.md`, `.claude/` | nothing |

A push to main starts every job. No branch rule requires a check, so
a pull request of the docs alone waits on nothing. A Go test that read
a file of `web/` or of the docs would not run on the pull request that
changes it, so none does: a file of `web/` a Go test needs goes in
`goread`, and so does the file that keeps the folder `web/embed.go`
embeds. `internal/workflows` reads every Go test and `web/embed.go` and
holds them to that.

## The speech library

Dictate hears with sherpa-onnx, through `sherpa-onnx-go`, which brings
the library as dylibs in the Go module cache, and cgo points the program
there. That folder exists on no Mac but the one that built it, so a build
from GitHub's Mac would not start on Tim's. `scripts/carry-libs.sh` copies
the dylibs to `bin/lib` and points the program at
`@executable_path/../Frameworks`, and `scripts/bundle-macos.sh` puts them
in the bundle's `Contents/Frameworks` and checks that every one resolves
there. The library is sherpa-onnx's release without speech synthesis,
`scripts/speech-libs.sh`, since the one with it carries espeak-ng, which
is GPL 3, and the bundle refuses it. As in FrameFairy.

The bundle's `Info.plist` says why the app wants the microphone, which
macOS asks Tim the first time Dictate is pressed. The app needs no
entitlement for it, since it is signed without the hardened runtime.

## The language model

Draft on a person writes a first message on the Mac with a local
language model, Gemma 4 26B A4B on Tim's, run by llama.cpp's
llama-server. No paid model and no key. The app carries its own
llama-server, beside the program in `Contents/MacOS`, because Tim has no
terminal step to install one.

`scripts/build-llama.sh` builds it from llama.cpp's source, at the
version and commit it pins, the same as FrameFairy's: one static file
with Metal built in, for Apple silicon rather than for the Mac that built
it, and nothing borrowed from that Mac, which the script checks. Its
licence, MIT, travels with it as `LICENSE-llama.cpp` in the bundle's
Resources. make builds it into `.build/llama` once and again only when the
script changed, by the checksum the stamp `.build/llama/built` keeps. The
build workflow and CI's `macos` job keep `.build/llama` in their cache by
the same rule, so a build takes minutes longer only after the script
changed.

`scripts/bundle-macos.sh` refuses a bundle without llama-server and signs
it ad hoc with the app. The release check, `cmd/speakertrail-release`,
knows which libraries it links: the system's Metal, MetalKit, Accelerate,
Foundation and CoreFoundation, and nothing else.

The model is downloaded in Settings, Language model, into the models
folder, or linked from FrameFairy's `~/.framefairy/models` when it is
there whole. Before llama-server starts, the model's needs are held
against the memory free now, and a server a crashed app left running is
stopped when the app starts, noted in `llama-servers` in the app's
folder. As in FrameFairy. `SPEAKERTRAIL_LLAMA_SERVER` names another llama-server, for a try
with a build of one's own.

## Elsewhere than on a Mac

Build runners and Claude Code cloud sessions run the same targets. There
make installs nothing, and `CI` in the environment turns installing off
everywhere. On Linux the app's window compiles against GTK 4 and
WebKitGTK 6, `libgtk-4-dev` and `libwebkitgtk-6.0-dev`, which
`scripts/cloud-setup.sh` installs in a cloud session. `make` there builds
`bin/speakertrail-app` and the tests run, but no window can be opened.
