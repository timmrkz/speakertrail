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
   branch pushed. A branch with no open pull request builds nothing, so
   opening or reopening a pull request builds it too: its branch is
   pushed before it is opened, and a pull request of that one push had
   no build and was not on the Updates page, as #93 was. A push builds
   only for a pull request open before it, and one opened at or after
   the push is built by its opening, so a commit is built once,
   `speakertrail-release build-plan`. A pull request opened in conflict with
   main, and a second commit pushed in the same second as the opening,
   wait for the next push. A
   commit that only changes the docs against the build there is gets no
   build of its own, see `scripts/needs-build.sh`. The build leaves a zip
   and nothing else. It gets no secret, because a branch runs its own
   copy of this file.
2. `.github/workflows/publish.yml` runs from main whenever a build starts
   or ends, whatever branch was built. It takes only runs of `builds.yml`
   started by a push or by a pull request opened or reopened, and starts
   a Mac only once a job on Linux found the zip the build left. It works out the channel, commit and version again
   from GitHub's own record of the run, and refuses a commit that is not
   in the history of main or of the pull request it is for. It checks
   what the app links and asks for with `cmd/speakertrail-release check`,
   in a step that has no key, and signs the app again with Tim's
   Developer ID and the hardened runtime, see
   [The Developer ID](#the-developer-id). `speakertrail-release pack`
   writes the build again in parts, see What an update downloads below.
   `sign` signs the zip and the stored zip with the key from the
   environment `updates`. The pieces and patches the release does not
   hold yet go up to the pre-release `dev`, then the zip, the stored zip
   packed, its parts and its entry. The commit that was built gets the
   status **Ready to update to** on the pull request.
3. `scripts/channel-list.sh` writes the channel list again from every
   entry, as `channels-a.json` and `channels-b.json` in `dev`. A pull
   request that closed leaves the list. Of the newest two hundred that are
   not open any more, the list names those merged as `merged` and those
   closed without a merge as `closed`. A newer commit whose build failed
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
- A pull request that was merged shows as merged, in amber, and one
  closed without a merge as closed, in red. One the list says neither of
  shows as not open, in amber. Nothing downloads until another channel is
  picked.
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

## The Developer ID

Every build Tim updates to is signed with his Developer ID and Apple's
hardened runtime, by `speakertrail-release harden` in `publish.yml`,
see `cmd/speakertrail-release/devid.go`. The
hardened runtime keeps other programs from putting their own code into
the app: macOS ignores the `DYLD_` settings it is started with, refuses
debuggers, and loads only libraries signed by Apple or by the same
developer. The app is signed as entitled to the microphone and nothing
else, `build/entitlements.plist`, because under the hardened runtime an
app without it hears only silence. macOS still asks Tim the first time.

A Developer ID also makes every build the same app to macOS. A build
signed ad hoc looks like a new app after each update, so macOS may ask
for the microphone again. The Exa key is no longer in the keychain, so
no update asks for it, see The first start in [BUILD.md](BUILD.md).

Builds made on the Mac with `make run` stay signed ad hoc, without the
hardened runtime, like Xcode's Sign to Run Locally. Under it an app signed
ad hoc may load no library but Apple's, and Dictate's speech library is
not Apple's.

Until `build/team-id.txt` names Tim's team, builds go out signed ad hoc,
as they are made, and the run says so. Once it does, a build is published
only when it is signed with that team's Developer ID, and a missing secret
fails the run on main and on the commit, never quietly. To set it up once:

1. Join the [Apple Developer Program](https://developer.apple.com/programs/enroll/)
   as an individual. It costs 99 € a year, and Apple may take a day or
   two to confirm it.
2. On the Mac, in **Keychain Access**, choose **Keychain Access,
   Certificate Assistant, Request a Certificate From a Certificate
   Authority**. Enter your email address, choose **Saved to disk**, and
   save the file.
3. In [Certificates](https://developer.apple.com/account/resources/certificates/add),
   choose **Developer ID Application**, upload that file, download the
   certificate and double-click it. It shows in Keychain Access under
   **My Certificates**, with the private key made in step 2 under it.
   Keychain Access may call it not trusted, since a Mac without Xcode
   lacks Apple's certificate between it and Apple's root. That changes
   nothing here, because `harden` fetches that one from Apple. Make it
   for Speaker Trail alone, and FrameFairy its own later, so either can
   be revoked without the other. Apple allows five. Both are still the
   same team to macOS, which trusts code by its team and what it is
   signed as, not by its certificate, see below.
4. In Keychain Access, right-click the certificate, **Export**, and save
   it as `developer-id.p12` on the Desktop with a new password. The file
   holds the private key too, which Apple cannot give out again, so it is
   the backup. Put it and its password in the password manager.
5. In Terminal, `base64 -i ~/Desktop/developer-id.p12 | pbcopy` puts the
   certificate on the clipboard as text.
6. On GitHub, in the environment `updates`, add two **environment
   secrets**: `SPEAKERTRAIL_DEVELOPER_ID` with the clipboard, and
   `SPEAKERTRAIL_DEVELOPER_ID_PASSWORD` with the password. Then delete
   `developer-id.p12` from the Desktop.
7. Give Claude the team ID, ten letters and digits under **Membership
   details** in the [account](https://developer.apple.com/account). Claude
   commits it in `build/team-id.txt`. It is not a secret: every app the
   team signs carries it.

The next build after that is signed with it. From then on
`speakertrail-release sign -team` refuses a build in which any program is
signed ad hoc, without the hardened runtime, or by another team, or
whose app is signed as anything but `com.speakertrail.app`, or any other
program or library as anything but its part of it.

Every build of a pull request is signed with the Developer ID too, so
Tim can try it. Its zip comes from the pull request's branch and is
unpacked on the Mac that holds the certificate, so `harden` trusts
nothing in it:

- It unpacks the zip itself, after `check` read it, and only files and
  folders inside `Speaker Trail.app`. A link, or a name that leads out of
  the .app, stops it before anything is written.
- It runs only `security`, `codesign` and `ditto`, from `/usr/bin`,
  never a program found through `PATH`.
- It reads both secrets once and takes them out of its environment, so
  no program it starts sees them. The release tool is built once, in a
  step before, without them and without the Go cache, which builds of
  third-party code write too. Every step that holds a key runs that one
  program, so `go` and its compiler never see either key. The key goes
  into a keychain made for the run, marked so it cannot be exported
  again, and the keychain is deleted at the end, also when signing
  fails.
- It takes Apple's certificate between the Developer ID and Apple's root
  only when it is signed by the Apple Root CA that macOS itself ships.
- It signs the app as `com.speakertrail.app`, whatever its `Info.plist`
  says, and each program and library in it as a part of it, by its
  file name, like `com.speakertrail.app.llama-server`, whatever an
  `Info.plist` built into it says. macOS gives code of the team the
  microphone and the keychain items of the app it is signed as, so no
  build, and nothing in one, can be signed as another of Tim's apps.
- It refuses a build with a program anywhere but `Contents/MacOS` and
  the libraries of `Contents/Frameworks`, which it signs itself. Such a
  program would keep the signature the branch gave it.
- Once signed, codesign checks every program and the app against
  Apple's certificate of Tim's team, not only the team the signature
  names, which is a string.

If a build signed this way does not open, the app cannot update away
from it. `make install` in a checkout puts a build made on the Mac back
in `/Applications`, and Updates works again from there. Removing
`build/team-id.txt` on main sends builds out signed ad hoc again.

The certificate runs out on the date Keychain Access shows, Tim's first
on 21 December 2027. A new one is made with steps 2 to 6. Every signature
carries Apple's time stamp, so a build signed before then stays good
after.

The builds are not notarised. Notarising is what lets a downloaded app
open past Gatekeeper on any Mac. The app downloads its updates itself,
and macOS asks Gatekeeper only of files a browser or mail marks as
downloaded, so nothing here needs it.

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
a program and cannot be read as one, unpacks to more than 1 GB, has a
file outside `Speaker Trail.app`, has an `Info.plist` that names
another app than `com.speakertrail.app`, or has an `Info-macos.plist` or
any other `Info-….plist`, which macOS would read in its place. The
stored zip `pack` wrote is signed only when it holds the same files, byte
for byte, as the zip `check` read. It may ask for the microphone, for
Dictate on Report a Problem, and be entitled to it, and to nothing else,
and it may carry sherpa-onnx's speech library in `Contents/Frameworks`
and llama-server, which drafts a first message, in `Contents/MacOS`, each
linking only what the list names. A
refused build says why on the commit, as a failed **Ready to update to**.
