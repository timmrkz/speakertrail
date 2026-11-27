# Updates reach the app sooner, and safer

Proposed, waiting for Tim's go. Nothing of it is built yet. Builds on
[2026-11-20-an-update-downloads-only-what-changed.md](2026-11-20-an-update-downloads-only-what-changed.md).

## The problem

Tim, 27 November 2026: "design a faster and safer update mechanism based
on the solution implemented in the FrameFairy repository. In particular,
I would like to understand which aspects are better here in terms of
what we are already doing and what's better in the FrameFairy
repository."

The same day, builds of pull requests stopped reaching Updates, because
the release `dev` held 1,000 files, GitHub's limit. #127 caps the patches
and makes room after a failed publish. #128 says four libraries that did
not change are signed again in every build.

Both apps update the same way at heart. Speaker Trail's updater is
FrameFairy's, ported, with the patches of the record above and Tim's
Developer ID on top. The app's side, `internal/updates`, is the same
code within a few lines, and so are the signatures, the claim, the two
copies of the channel list and the cache. Where they differ is how a
build gets from its build job onto the release, and what the release
holds.

### Speed

Read from GitHub's record of the 25 newest builds of pull requests that
turned Ready to update to, in each repository, 25 to 27 November 2026.
From the moment the build job started to the status Ready to update to,
which the publish sets once the build's entry is up. Updates does not
wait for CI, so this is most of the time from a push to the build on
Updates. It leaves out the job `which` before the build, about 20 s in
both. Median of 25 runs, and the 90th percentile, which is the spread.

| From the build's start (s) | Speaker Trail | FrameFairy | FrameFairy against Speaker Trail |
| --- | ---: | ---: | --- |
| the build | 72 | 167 | 132 % slower |
| the build's end to Ready to update to | 301 | 45 | 85 % faster |
| in all, median | 366 | 211 | 42 % faster |
| in all, 90th percentile | 670 | 256 | 62 % faster |

Three of Speaker Trail's 25 took 1,578 to 2,578 s, on the full release
before #127. They are in the 25 and in the percentile.

Speaker Trail builds faster, because its app is smaller. It publishes six
times slower. The publish job, step by step, median of the 15 newest
publishes that uploaded a build:

| Step (s) | Speaker Trail | FrameFairy | what differs |
| --- | ---: | ---: | --- |
| the release tool | 16 | 7 | built without a cache, against downloaded once, see below |
| check and Developer ID | 5 | 1 | FrameFairy signs ad hoc, so it has nothing to sign |
| pack in parts | 98 | 8 | Speaker Trail makes patches, FrameFairy none |
| upload | 130 | 10 | a `gh` call a file in shell, against Go |
| the whole job | 273 | 41 | |

Since #127 split Speaker Trail's upload in three steps, two publishes
have run. The build went up in 19 and 17 s, the patches in 101 and 76 s,
and the entry in 2 s. The entry waits for every patch, though an app
that finds a patch missing takes the file's piece instead,
`internal/updates/assemble.go`. Both jobs waited about 5 s for a runner, and
Speaker Trail's at most 145 s, since its publish runs on a Mac.

What the pack step spends its 98 s on shows in its log. The publish of
27 November at 19:15 made 60 patches: 12 of the app program, from 120 to
510 KB, and 48 of `llama-server`, `libonnxruntime` and the two
sherpa-onnx libraries, from 1.7 to 3.2 KB each. A patch of 2 KB between
two copies of an 18 MB library is a library whose code did not change and
whose signature did, as #128 suspects. To make each of those 48, the job
downloaded the library from twelve builds and ran bsdiff over it. So most
of the 98 s, most of the patches' upload, and four of the five pieces
each build uploads, are #128. The review of this record took the
`llama-server` of three builds apart: the bytes before the signature and
the code directory are the same, and only the signature's CMS blob
differs.

### Download size

A usual update, for a pull request of a few hundred lines:

| What the app downloads (MB) | Speaker Trail | FrameFairy |
| --- | ---: | ---: |
| a usual update, with a patch | 0.12 to 0.51 | none, the piece instead |
| a usual update, the program's piece | 5.1 | about 7 |
| an app with none of its files | 16.9 | about 33 |

Speaker Trail's from the log above and the parts of #129's build, FrameFairy's from its
UPDATES.md. Here Speaker Trail is ten to forty times smaller, by its
patches and its lean programs.

### Storage

What each release `dev` holds, read on 27 November 2026 at about 19:25 UTC:

| | Speaker Trail | FrameFairy |
| --- | ---: | ---: |
| files | 532 | 46 |
| of them patches | 419 | 0 |
| of them pieces | 80 | 23 |
| MB | 502 | 676 |

GitHub allows 1,000 files a release, each under 2 GB, and no limit on
the total. FrameFairy is nowhere near the limit, because it has no
patches. Speaker Trail's patches are 79 % of its files, and in the publish
above 48 of 60 were the libraries of #128.

### Safety

| | Speaker Trail | FrameFairy |
| --- | --- | --- |
| the zip signed with the update key, checked by Wails before unpacking | yes | yes |
| the claim, so a build is offered only as what it was signed as | yes | yes |
| only main's publish holds the key, a branch's commit checked against its history | yes | yes |
| a branch's zip read for what it could do before the key is there | yes | yes |
| a pull request opened after its push still gets a build | yes, #98 | no |
| Tim's Developer ID and the hardened runtime | yes | no, ad hoc |
| the release tool built on main once, attested, and checked before a job with a write token runs it | no, built in the job, and `go run` with main's Go cache in the list job, #55 | yes |
| every failed publish says why on its commit | only a refusal of check, harden, pack or sign | every failure, `not-published` |
| every decision of the publish in Go, with tests | partly: `build-plan`, `pack`, `sign`, `unreachable`. The rest is shell in `publish.yml` and `scripts/channel-list.sh`, #57 | yes |
| a build that starts but does not work is undone | no | no |
| an older build of the channel followed refused by the app | no | no |
| a download cut off goes on where it stopped | no, starts again | no, starts again |

Neither can undo a broken build. Wails' helper, in `pkg/updater/helper.go`
of the version pinned, puts the old app back only when the new one
cannot be started at all, and removes it the moment the new one runs.
Speaker Trail's own step on quit, `install.go`, keeps nothing either. A
build that opens and then crashes, or whose Updates page is broken,
stays until `make install` is run in a checkout. UPDATES.md says so.

Neither app refuses an older build of the channel it follows. The claim
holds the channel, version, commit, size and SHA-256, but not when the
build was made, and the app takes whatever build its channel has when it
is not the one running. Whoever can write the release can put back an
older, signed entry of main, one with a bug that was fixed since, and
the app takes it. Writing the release takes a token that can write the
repository, which a branch's own copy of `builds.yml` can ask for.

A cut download starts again from nothing in both. At Speaker Trail's
sizes that is seconds, so this design leaves it.

## Five simulations

1. **Who may know about whom.** Each job holds one key and knows only
   what it needs. A job on a Mac checks the zip and signs it with the
   Developer ID, and hands the signed zip on. A job on Linux packs it,
   signs it with the update key and uploads it. Both run the release
   tool main built once and GitHub attested, FrameFairy's
   `release-tool.yml` and its action. The app trusts the order of builds
   only from a signature bound to the claim, over the time the build
   was made.
   Cost: two jobs where there is one, about 10 s to hand the zip on, two
   workflows ported. The Mac is held for about 30 s instead of 270.
2. **What changes together.** A file changes when its code changes, and
   not when it is signed again. `harden` signs as today, then compares
   each library with the same library of the builds on the list, main's
   and the channel's first. Where the bytes up to the signature's CMS
   blob are the same, so the code directory with its code, identifier,
   entitlements and requirements, and the CMS blob's certificate is the
   one `harden` signs with now, it keeps that build's signed bytes. The
   code directory does not cover the certificate, hence the second
   condition, so a renewed or revoked certificate is never carried on.
   The kept file is checked with `codesign --verify --strict` against
   Tim's team like any other, before the app's own seal is made.
   Then a build has one changed program, the app's, one piece and one
   patch a base. Cost: one download of the libraries a publish, about 11
   MB. Fixes #128.
3. **Where state lives.** The release holds one file of patches a build,
   and the app reads the one patch it needs with an HTTP range. Files
   per build fall from up to 60 to 4. And the app keeps the build it
   replaced, about 56 MB, until the new one has said it started, so a
   build that does not start can be undone. Cost: range requests through
   GitHub's redirect to its storage, to prove first, and a format the
   parts must say.
4. **What runs together.** A build goes on Updates as soon as it is
   signed: its pieces, its zips and its parts, then its entry and the
   status, and its patches after. The parts already name the patches. An
   app that updates in that minute finds a patch missing and takes the
   file's piece, 5.1 MB, as `assemble.go` already does. Cost: a reorder
   of two steps in `publish.yml`, and a larger download for an update in
   that minute.
5. **What a test has to build.** Every step of the publish is one
   command of the release tool, as in FrameFairy: `publish` uploads in
   Go, side by side, a piece that is there already stays, `not-published`
   says on the commit why any step failed, and `channels` writes the list
   and leaves out a commit whose build ended without one. Each is tested
   against a fake GitHub, FrameFairy's `github.go`. The step on quit
   gains a test that a build which never says it started is undone.
   Cost: `scripts/channel-list.sh` and the shell in `publish.yml` go,
   part of #57.

## What converges

- **Signed bytes that change only with the code**, in 2, and 1, 3 and 4
  need less once it holds: with one changed program a build, the pack
  bsdiffs about a quarter of what it does today, about 25 s, the release
  holds a fifth of the files, and a Mac is held briefly enough that
  splitting the job buys little.
- **The entry before the patches**, in 4, kept though only one needs it,
  because it is a reorder with no new format and saves about 100 s today.
  It comes first.
- **The publish in Go, FrameFairy's commands**, in 1 and 5. It is
  FrameFairy's upload, ten times quicker than a `gh` call a file, and
  the no-shell rule.
- **The release tool built once and attested**, in 1 and 5. Built in
  the job without a cache, FrameFairy's publish took 52 s, and with the
  tool downloaded it takes 41. It saves about 10 s here. FrameFairy
  reworked it in three records in two days, so it is not small. #55 is
  closed sooner by two lines: the list job builds the tool with
  `cache: false` and `go build`, as the publish job already does.
- **The build before kept, and a build that does not start undone**, in
  3 and 5. Neither app has it.
- **The time of the build, signed with the claim**, in 1. The app never
  steps back within the channel it follows by itself. A pick of another
  channel still goes sideways, as now.

Dropped, each needed by one simulation only:

- The split into a Mac job and a Linux job, of 1. Once #128 is fixed the
  Mac job is about a minute. If the Mac queue still shows in the times,
  it comes back.
- The file of patches read by range, of 3. With one patch a base the
  release holds about 120 files, and the range adds a request the app
  has never made, through a redirect GitHub may change.
- `rcodesign`, a signer that runs on Linux, which would take the Mac out
  of the publish. It is a second program in the path of the Developer ID,
  where Apple's `codesign` is the reference, and the check that every
  program is Tim's team's would still want a Mac.

Kept as it is, because Speaker Trail's is better: the patches, the lean
programs, the Developer ID, the build on a pull request's opening, the
`built` job that starts no Mac for a run with nothing to publish, and
the merged and closed marks.

## The decision

The patches stay, and go up after the entry. Signed bytes change only
when the code does, the publish is FrameFairy's commands in Go with its
attested release tool, the build before guards the new one, and each
entry signs when its build was made.

```
publish.yml             one job on a Mac, every step one command of the tool, the entry
                        and the status before the patches
.github/actions/        release-tool: main's build of the tool, attested, from FrameFairy
cmd/speakertrail-release
                        harden: keeps a library's signed bytes when its bytes up to the CMS
                        blob are a listed build's and its certificate is today's
                        bases: the channel's build before, main's, the other listed builds,
                        the newest three that left the list
                        publish, not-published, channels: from FrameFairy
                        sign: a second signature, over the claim and the time the build
                        run was created
                        publish: refuses to replace an entry of a newer build
cmd/speakertrail-app    install.go: starts the new program once to check it loads, keeps the
                        build it replaces and the app's file, opens the new one, and puts
                        both back when the new one does not say it started within a
                        minute. Relaunch goes through it too, not Wails' helper
internal/updates        refuses, unasked, a build of the channel followed older than the one
                        running
Wails' pkg/updater      downloads, checks the signature, unpacks, as before
```

Each layer uses only the ones under it, as in the record before.

What a build that does not start looks like. Before it swaps, the old
build's step, on Relaunch and on quit alike, starts the new program once
in a mode that loads its libraries and ends. A library macOS refuses
under the hardened runtime ends a program before any Go code runs, so
only a program of the old build can see that. When it fails, the old
app stays. When it passes, the step copies `speakertrail.json` and
`profile-texts.json` aside, moves the old app aside and the new one in.
The new build writes a mark into the app's folder once the window has
loaded the interface and the workspace answered. On Relaunch the old
build's step waits up to a minute for it. Without it, the step ends the
new build, puts the old app and the app's files back, opens it, and
writes down the SHA-256 of the build that failed. The app then does not
install that build by itself again, and Updates says it did not start,
with the build before running. A build put in on quit is opened later,
with nobody waiting. So each start first looks whether the start before
it, of the same build, said it started. When it did not, it puts the
old app and the files back the same way, after the second failed start.
Once the new build said it started, the old app and the copies go.

The app's files go back with the app because an older build drops the
fields it does not know when it writes, and refuses a file of a newer
`store.Version` outright. A build that failed may have written the file
before it failed.

What the time is. A run of an older commit run again, as a cancelled
`macos` job is, is published today as its channel's build, since its
commit is in the channel's history, and replaces the newer entry. So the
time is when the build's run was created, which a run again keeps, not
when it was published. The second signature is over the line
`speakertrail build order 1`, the claim's own lines and that time, so it
cannot be moved to another entry. The publish refuses to replace an
entry whose time is newer, and the app refuses, unasked, an entry of the
channel it follows older than the build it runs.

## What follows

Estimates, read from the steps above, not measured:

| | today | after | change |
| --- | ---: | ---: | --- |
| publish job up to the entry (s) | 273 | about 80 | about 71 % faster |
| build start to Ready to update to, median (s) | 366 | about 165 | about 55 % faster |
| files a build adds to `dev` | about 69 | about 14 | about 80 % fewer |
| files on `dev` | 532 | about 120 | about 77 % fewer |
| a usual update (MB) | 0.12 to 0.51 | the same | 0 % |
| an app with only the libraries of an earlier build (MB) | 16.9 | 5.1 | 70 % smaller |

The pull requests that build it measure each line as these were
measured, from GitHub's record of 25 builds.

- It costs: about 12 MB downloaded by every publish to compare the
  libraries, a 5.1 MB piece for an update in the minute before the
  patches are up, a mark the app writes on each start, a start of the
  new program before each swap, a Relaunch that waits up to a minute,
  about 56 MB of disk for the old app until the new one started, and a
  second signature in each entry. The time goes beside the claim, not into it, so the apps
  from before still check the claim they know. An entry without the
  time is not installed by itself any more, only picked.
- It buys: a build on Updates about three minutes sooner, every 25
  builds about 80 minutes, a release that stays far from its
  limit without a cap doing the work, a broken build undone without a
  terminal, and no way to hand the app an older build of its channel.
- It was wrong if a build that changed no library still uploads a
  library's piece, if a build still takes over two minutes from the end
  of its build to Ready to update to, or if a build that started fine is
  put back.

### For FrameFairy

The undoing and the signed time make Speaker Trail's updater differ
from FrameFairy's, where CLAUDE.md asks a port to copy it. They are
built here first, because only here does a build go out with a
Developer ID that can fail to load, and that is Tim's to agree.
FrameFairy could take them from here later, with the patches, the lean
programs and the build on a pull request's opening, by a briefing for
its sessions once Tim says so.

### Issues, once Tim says go

In this order, the cheapest large gain first:

1. The entry and the status go up before the patches, as in
   simulation 4. Size small.
2. #128, signed bytes that change only with the code, as in simulation 2.
   Size medium.
3. #55 closed in the list job, `cache: false` and `go build`. Size small.
4. The bases: main's and the channel's build before first, the other
   listed ones, and the newest three that left. Size small.
5. A build that does not start is undone, with the app's files, on
   quit and on Relaunch. Size medium.
6. Each entry signs when its build run was made, bound to the claim, the
   publish never replaces a newer entry, and the app does not step back
   within its channel by itself. Size small.
7. The publish in Go, `publish`, `not-published` and `channels` from
   FrameFairy, part of #57. Size medium.
8. The release tool built once on main and attested, from FrameFairy.
   Size medium.
