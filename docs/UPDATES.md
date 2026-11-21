# Updates

Speaker Trail lives in `/Applications` and updates itself. Every push to
main and to an open pull request builds the app on GitHub. The app's
Updates page lists main and the open pull requests, by what they are,
Branch main and PR #44, as FrameFairy's does. Pick one, and the app
downloads its newest build by itself. How updates work is behind the
info mark beside the list. **Relaunch** restarts the app into
it, and if the app is quit instead, the build goes in on the way out. No
terminal, no `git pull`, no `make run`.

It is FrameFairy's update mechanism, ported. FrameFairy's
`docs/UPDATES.md` has the history of every decision behind it.

## Installing it the first time

Once, on the Mac, in a checkout of this repository:

1. `make update-key`. It makes the key builds are signed with. The
   private half goes on the clipboard, the public half into
   `cmd/speakertrail-app/update-key.txt`, and both are printed.
2. On GitHub, in the repository's **Settings, Environments**, make an
   environment called `updates`. Under **Deployment branches and tags**
   choose **Selected branches and tags** and add `main`, so no other
   branch can enter it. Add an **environment secret** called
   `SPEAKERTRAIL_UPDATE_KEY` and paste the private half. Paste it into the
   password manager too. Nowhere else, never into a chat.
3. Give the public half to Claude, who commits it. It is not a secret.
4. Once that is merged: `git pull && make install`. The app is now in
   `/Applications` and opens from Spotlight.
5. Open **Updates** in the app and pick main or a pull request.

A build made on the Mac follows nothing until a channel is picked. From
then on every build comes from GitHub.

## What happens on a push

1. `.github/workflows/builds.yml` builds the app on a macOS runner, for
   the push's channel: `main`, or `pr-9` for the open pull request of the
   branch pushed. A branch with no open pull request builds nothing. A
   commit that only changes the docs against the build there is gets no
   build of its own, see `scripts/needs-build.sh`. The build leaves a zip
   and nothing else. It gets no secret, because a branch runs its own
   copy of this file.
2. `.github/workflows/publish.yml` runs from main whenever a build starts
   or ends, whatever branch was built. It takes only runs of `builds.yml`
   started by a push. It works out the channel, commit and version again
   from GitHub's own record of the run, and refuses a commit that is not
   in the history of main or of the pull request it is for. It checks
   what the app links and asks for with `cmd/speakertrail-release check`,
   in a step that has no key. `speakertrail-release pack` writes the
   build again in parts, see What an update downloads below. `sign`
   signs the zip and the stored zip with the key from the environment
   `updates`. The pieces and patches the release does not hold yet go up
   to the pre-release `dev`, then the zip, the stored zip packed, its
   parts and its entry. The commit that was built gets the status
   **Ready to update to** on the pull request.
3. `scripts/channel-list.sh` writes the channel list again from every
   entry, as `channels-a.json` and `channels-b.json` in `dev`. A pull
   request that closed leaves the list. A newer commit whose build failed
   is not said to be on its way. `speakertrail-release unreachable` then
   names, and gh removes, an hour after they went up, the zips and packed zips no entry points
   at, the parts of every build that is neither listed nor one of the
   newest eight that left the list, and the pieces and patches no
   remaining parts name.

A build takes a few minutes after the push. A version reads like
`0.1.0-pr9.e808a87`: the app's version, the channel and the commit.

Until the pull request that adds this is merged, `publish.yml` is not on
main, so nothing is published yet.

## What an update downloads

Only what changed, and of a file that changed, only the difference.

- **The programs ship lean.** The app program is built without its
  debug information and without the commit Go stamps in, `-s -w
  -buildvcs=false`, and the speech libraries lose their local symbols,
  `strip -x` in `scripts/carry-libs.sh`. A build whose code did not
  change then has the same bytes as the one before.
- **Each file is a piece.** `pack` writes the build again as a zip that
  stores its files as they are, and beside it its parts: that zip in
  order, as its few bytes of headers and each file named by its SHA-256.
  Each file is published once, packed with zstd, as a piece named by its
  SHA-256, whichever build holds it.
- **A changed file is a patch.** For each file that changed, `pack` also
  makes a patch, with bsdiff, against the same file of the builds an app
  may run, picked in `cmd/speakertrail-release/release.go`: every build
  the channel list holds, and the
  newest eight that left it, such as the last build of a pull request
  just merged. The parts name each patch by the SHA-256 of the file it
  starts from. A patch is kept only where it weighs less than the piece.
  A build older than the one running was published before it, so
  picking it takes the piece.
- **The app puts the build together.** It writes the stored zip from the
  files it already has with the same bytes, a file made from its own and
  a patch where there is a patch from it, and a downloaded piece for the
  rest, `internal/updates/assemble.go`. It applies a patch with code of
  its own, `patch.go`, and checks what it makes against the file's
  SHA-256. Wails' updater then checks the whole zip against its signature
  as it checks any.
- **What can go wrong costs time, never the update.** A patch that cannot
  make its file costs that file's piece. When the parts do not work, a
  piece is gone, or what the app lacks weighs more than the stored zip
  packed with zstd, as for an app built on the Mac, the app downloads
  that packed zip instead, `whole.go`. The deflated zip stays at its
  address, signed as before, for the apps from before parts.

What that weighs, measured once on the cloud machine: on main's build of
20 November and #38's, their programs stripped with `llvm-strip` the way
the build now strips them, and for the 605 lines, on Linux builds made at
both commits, because `dev` keeps only the newest Mac build of each
channel. A size packed the same way is the same every time.

| What changed | today (MB) | after (MB) | change |
| --- | ---: | ---: | --- |
| nothing in the app, a docs or release tool change | 19.9 | 0.002 | 100 % smaller |
| a pull request of 605 lines of Go and interface | 19.9 | 0.24 | 99 % smaller |
| a new feature of thousands of lines | 19.9 | 0.79 | 96 % smaller |
| a build no patch starts from: the app program's piece | 19.9 | 4.7 | 76 % smaller |
| an app built on the Mac: the packed zip | 19.9 | 11.5 | 42 % smaller |

The kept build in `~/Library/Caches/Speaker Trail/builds` is the stored
zip, about 34 MB. The first update after this arrived is still the whole
zip, and the first build published after it has no patches yet, since no
build on `dev` had parts to patch against. See
[the decision record](decisions/2026-11-20-an-update-downloads-only-what-changed.md).

## What the app does

`cmd/speakertrail-app/updates.go` and `internal/updates`:

- It reads the channel list when it starts, when a channel is picked and
  every ten minutes, every twenty seconds while a newer commit is being
  built.
- When the channel followed has a build that is not the one running, it
  puts it together from its own files, patches and pieces into
  `~/Library/Caches/Speaker Trail/builds`, see What an update downloads,
  checks the signature against the public half built into it, and has it
  ready. The app's log says what each update kept, patched and fetched.
- **Relaunch** waits while a run goes, so a run is never stopped half
  way. The engine refuses a new run while the app relaunches.
- **Relaunch** starts the app's own program once more, as Wails'
  updater: it waits for the app to quit, puts the new build in place and
  opens it. Started so, `main` does that step before anything of the
  app: it opens no log and no app's file and loads no language model.
- A pull request that closed shows as closed, and nothing downloads until
  another channel is picked.
- What it picked is in `updates.json`, in the app's folder beside
  `speakertrail.json`.

Where things stand reaches the interface the way it does in FrameFairy:
the app sends it as the Wails event `updates` every time it changes, and
the Updates page and the sidebar's dot show what it last sent. The
interface asks `/api/updates` only when the workspace starts, when the
Updates page opens, and after a pick the app refused. A click is sent as
a request:

| Request | What it does |
| --- | --- |
| `GET /api/updates` | where things stand, see `UpdateState` |
| `POST /api/updates/check` | looks for a newer build now |
| `POST /api/updates/follow` with `{"channel": "pr-9"}` | follows a channel |
| `POST /api/updates/relaunch` | quits into the build that is ready, 409 while a run goes |
| `POST /api/updates/commit` | opens the running build's commit on GitHub |

## The key

An Ed25519 pair, made once with `make update-key`. The private half lives
only in the GitHub environment `updates`, which only main can enter, and in
the password manager. The public half is in the repository and built into
the app, which is how the app checks a download without asking anybody.

Losing the private half costs a new pair and one `make install` by hand.
`make update-key` refuses to replace a key that is there, because every
build made with the old one trusts only the old one.

## Guarding main

The key is only as safe as main, because whatever reaches main runs with
it. Two settings guard main, in the repository's **Settings, Rules,
Rulesets**. Tim sets them once:

- A branch ruleset for `main`: **Restrict deletions**, **Block force
  pushes** and **Require a pull request before merging**.
- A tag ruleset for every tag, `~ALL`: **Restrict creations**, with Tim
  as the only one allowed past it, in **Bypass list**.

publish.yml already refuses a build whose commit is not in main's
history or its pull request's, so these are a second wall, not the
first.

## What is never signed

`speakertrail-release check` reads every file in the zip, and `sign`
reads it all again before it signs, see
`cmd/speakertrail-release/inputs.go`. They refuse a build that links a
library not on its list, in any of the ways a program can link one, has
a program that sets where macOS's loader looks, or that looks for the
libraries it links as `@rpath` anywhere but inside the .app, holds a
link, has a file that names what taps keys, records the screen, opens
the camera or asks for administrator rights, has a file that looks like
a program and cannot be read as one, or unpacks to more than 1 GB. The
stored zip `pack` wrote is signed only when it holds the same files, byte
for byte, as the zip `check` read. It may ask for the microphone, for Dictate on Report a
Problem, and for nothing else, and it may carry sherpa-onnx's speech
library in `Contents/Frameworks` and llama-server, which drafts a first
message, in `Contents/MacOS`, each linking only what the list names. A
refused build says why on the commit, as a failed **Ready to update to**.
