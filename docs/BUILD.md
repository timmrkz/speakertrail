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
builds llama-server and ffmpeg, which take a few minutes each, see
[The language model](#the-language-model) and [ffmpeg](#ffmpeg), the
interface and the app,
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

The first build signed with the Developer ID asks for the keychain
password once, the first time it reads the key. Always Allow there
covers every build signed with it after, until the key is saved again
in Settings, see The Developer ID in
[UPDATES.md](UPDATES.md). A build signed ad hoc, like one from `make
run`, asks once for itself. Closing the box without an answer keeps the
key back for that one read, and the next run or draft asks again. Keychain
Access shows the item as open to all applications, because the list of
trusted apps would ask a second time. Another program is still asked, by
the item's partition list, which holds only the builds you allowed.

## Where things are kept

| What | Where |
| --- | --- |
| People, searches, runs, settings | `~/Library/Application Support/Speaker Trail/speakertrail.json` |
| The text of each person's profile, as Exa's index had it | `~/Library/Application Support/Speaker Trail/profile-texts.json` |
| The Exa key | the keychain, as **Speaker Trail: Exa API key** |
| The channel the app follows | `~/Library/Application Support/Speaker Trail/updates.json` |
| Builds downloaded to update to | `~/Library/Caches/Speaker Trail/builds` |
| The speech models Dictate hears with, and the language model that drafts first messages | `~/Library/Application Support/Speaker Trail/models` |
| llama-server, built once by make | `.build/llama` in the repository, never committed |
| ffmpeg and ffprobe, built once by make | `.build/ffmpeg` in the repository, never committed |
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
| `make ffmpeg` | builds ffmpeg and ffprobe, when they are not built or `cmd/speakertrail-release/ffmpeg.go` changed. `make` does it by itself on a Mac |
| `make install` | the same, copied to `/Applications`, where it updates itself |
| `make update-key` | makes the key builds are signed with, once, see [UPDATES.md](UPDATES.md) |
| `make carry-over` | the people connected in the Docker version, into the app, see above |
| `make mock` | the interface alone in a browser with invented data, on http://localhost:5173 |
| `make test` | `lint`, `unit`, `interface` and `walks` |
| `make lint` | `gofmt`, `go vet` and golangci-lint with the rules in `.golangci.yml` |
| `make unit` | every Go test under the race detector, and the test of `scripts/needs-build.sh`. Go's test cache is on, so what did not change is not tested again |
| `make interface` | a type check of the interface and its tests, with vitest |
| `make walks` | the interface driven in Chromium with invented data, the walks in `web/walks/`, side by side |
| `make check` | what this machine has and still needs |
| `make clean` | removes what make built. The app's file stays, and so do llama-server and ffmpeg in `.build/`, which take minutes to build again |
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

Every job has a limit, five minutes, and `macos` 30 for the time a new
llama-server and ffmpeg take to build. The `go` job installs Ubuntu's
ffmpeg, and `macos` puts the one the app carries first on the path, for
the tests that attach a screen recording to a report. Go's modules are kept in GitHub's cache by
`go.sum`, and what Go built and tested, and what golangci-lint found, by
main's last run that passed: a pull request starts from there and builds
and tests again only the packages it changed, and those with a test that
reads a file of the repository, since Go knows a file by when it changed
and a checkout makes every file new. Only main saves them. `macos` runs
the keychain's tests every time, because the cache cannot see the Mac's
keychain. The walks need Playwright's headless shell of Chromium alone.

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
macOS asks Tim the first time Dictate is pressed. A build made here is
signed ad hoc, without the hardened runtime, and needs no entitlement for
it. A build Tim updates to is signed again with his Developer ID, the
hardened runtime and the microphone entitlement, see
[UPDATES.md](UPDATES.md#the-developer-id).

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
stopped when the app starts. Every server is noted in `llama-servers` in
the app's folder the moment it starts, before its model has loaded. As in FrameFairy. `SPEAKERTRAIL_LLAMA_SERVER` names another llama-server, for a try
with a build of one's own.

## ffmpeg

Report a Problem takes screen recordings, and the ffmpeg and ffprobe the
app carries read them: what a file is, its size and length, a copy
without its place, date or device, and a still of its first frame, see
`cmd/speakertrail-app/recording.go`. They lie beside the program in
`Contents/MacOS`, the only place the app takes them from, as
`internal/carried` finds llama-server. `SPEAKERTRAIL_FFMPEG` and
`SPEAKERTRAIL_FFPROBE` name others, for a try.

`speakertrail-release ffmpeg`, in `cmd/speakertrail-release/ffmpeg.go`,
builds them from ffmpeg's source, the release and commit it pins, taken
from git. It is FrameFairy's `build-ffmpeg.sh` at the same pin, written in
Go, since this repository has no new shell scripts, and without libass,
freetype, fribidi and harfbuzz, which FrameFairy needs only to burn
captions. Its configure line is FrameFairy's otherwise: no
`--enable-gpl`, so no libx264 and the build is LGPL, only files and pipes
as ways in and out, no network, none of the formats that read other files
a recording names, and Apple's VideoToolbox and AudioToolbox on a Mac. It
reads each program back and stops when one says GPL, has libx264, reads
through any other protocol or format of that kind, or needs a library of
the Mac that built it. The LGPL's text travels with them as
`LICENSE-ffmpeg.txt` in the bundle's Resources, and `BUILD-ffmpeg.txt`
beside it says what the build is, read off the binary: the commit of
ffmpeg's source and the configure line. FrameFairy also puts an archive
of that source on its release, which Speaker Trail does not yet.

make builds them into `.build/ffmpeg` once, and again only when
`ffmpeg.go` changed, by the checksum the stamp `.build/ffmpeg/built`
keeps. The build workflow and CI's `macos` job keep `.build/ffmpeg` in
their cache by the same rule. The release check knows which libraries
they link, see `ffmpegLinks` in `cmd/speakertrail-release/inputs.go`, and
`scripts/bundle-macos.sh` refuses a bundle without them and signs them ad
hoc with the app.

## Elsewhere than on a Mac

Build runners and Claude Code cloud sessions run the same targets. There
make installs nothing, and `CI` in the environment turns installing off
everywhere. On Linux the app's window compiles against GTK 4 and
WebKitGTK 6, `libgtk-4-dev` and `libwebkitgtk-6.0-dev`, which
`scripts/cloud-setup.sh` installs in a cloud session. `make` there builds
`bin/speakertrail-app` and the tests run, but no window can be opened.
