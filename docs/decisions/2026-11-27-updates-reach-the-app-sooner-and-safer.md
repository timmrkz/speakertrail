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
wait for CI, so this is the time from a push to the build on Updates,
less the few seconds before a runner takes the build. Median of 25 runs,
and the 90th percentile, which is the spread.

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

The upload of Speaker Trail is 36 s in the one publish since #127 split
it in three steps, one run. Both jobs waited about 5 s for a runner, and
Speaker Trail's at most 145 s, since its publish runs on a Mac.

What the pack step spends its 98 s on shows in its log. The publish of
27 November at 19:15 made 60 patches: 12 of the app program, from 120 to
510 KB, and 48 of `llama-server`, `libonnxruntime` and the two
sherpa-onnx libraries, from 1.7 to 3.2 KB each. A patch of 2 KB between
two copies of a 29 MB library is a library whose code did not change and
whose signature did, as #128 suspects. To make each of those 48, the job
downloaded the library from twelve builds and ran bsdiff over it. So most
of the 98 s, and four of the five pieces each build uploads, are #128.

### Download size

A usual update, for a pull request of a few hundred lines:

| What the app downloads (MB) | Speaker Trail | FrameFairy |
| --- | ---: | ---: |
| a usual update, with a patch | 0.12 to 0.51 | none, the piece instead |
| a usual update, the program's piece | 4.7 | about 7 |
| an app with none of its files | 16.9 | about 33 |

Speaker Trail's from the log above and UPDATES.md, FrameFairy's from its
UPDATES.md. Here Speaker Trail is ten to forty times smaller, by its
patches and its lean programs.

### Storage

What each release `dev` holds, read on 27 November 2026 at 19:30 UTC:

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
   only from a signature beside the claim, over the time the build was
   published.
   Cost: two jobs where there is one, about 10 s to hand the zip on, two
   workflows ported. The Mac is held for about 30 s instead of 270.
2. **What changes together.** A file changes when its code changes, and
   not when it is signed again. `harden` signs as today, then compares
   each library's code directory hash with the same library of the
   channel's build before. Where they match, it keeps that build's signed
   bytes. The hash covers the code, the identifier, the entitlements and
   the requirements, so the same hash is the same signed code, and the
   kept file is checked by codesign against Tim's team like any other.
   Then a build has one changed program, the app's, one piece and one
   patch a base. Cost: one download of the libraries a publish, about 11
   MB. Fixes #128.
3. **Where state lives.** The release holds one file of patches a build,
   and the app reads the one patch it needs with an HTTP range. Files
   per build fall from up to 60 to 4. And the app keeps the build it
   replaced for a day, in `~/Library/Caches/Speaker Trail/builds`, so a
   build that does not start can be undone. Cost: range requests through
   GitHub's redirect to its storage, to prove first, and a format the
   parts must say.
4. **What runs together.** A build goes on Updates as soon as it is
   signed, with its pieces, and its patches follow in a second step that
   writes its parts again. An app that updates in that minute downloads
   the program's piece, 4.7 MB. Cost: parts written twice, and GitHub
   answers 404 for a file while it is replaced, which the channel list
   needed two copies for.
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
  need less once it holds: with one changed file a build, the pack takes
  seconds, the release holds a fifth of the files, and a Mac is held
  briefly enough that splitting the job buys little.
- **The publish in Go, FrameFairy's commands**, in 1 and 5. It is
  FrameFairy's upload, ten times quicker than a `gh` call a file, and
  the no-shell rule.
- **The release tool built once and attested**, in 1 and 5. Built in
  the job without a cache, FrameFairy's publish took 52 s, and with the
  tool downloaded it takes 41. It also takes main's Go cache out of
  every job that holds a write token, #55.
- **The build before kept, and a build that does not start undone**, in
  3 and 5. Neither app has it.
- **The time of the build, signed beside the claim**, in 1. The app never steps back
  within the channel it follows by itself. A pick of another channel
  still goes sideways, as now.

Dropped, each needed by one simulation only:

- The split into a Mac job and a Linux job, of 1. Once #128 is fixed the
  Mac job is about a minute. If the Mac queue still shows in the times,
  it comes back.
- The file of patches read by range, of 3. With one patch a base the
  release holds about 120 files, and the range adds a request the app
  has never made, through a redirect GitHub may change.
- The patches after the entry, of 4. They save about 15 s once #128 is
  fixed, for parts written twice and a window of 404.
- `rcodesign`, a signer that runs on Linux, which would take the Mac out
  of the publish. It is a second program in the path of the Developer ID,
  where Apple's `codesign` is the reference, and the check that every
  program is Tim's team's would still want a Mac.

Kept as it is, because Speaker Trail's is better: the patches, the lean
programs, the Developer ID, the build on a pull request's opening, the
`built` job that starts no Mac for a run with nothing to publish, and
the merged and closed marks.

## The decision

The patches stay. Signed bytes change only when the code does, the
publish is FrameFairy's commands in Go with its attested release tool,
the build before guards the new one, and each entry signs when its build
was published.

```
publish.yml             one job on a Mac, every step one command of the tool
.github/actions/        release-tool: main's build of the tool, attested, from FrameFairy
cmd/speakertrail-release
                        harden: keeps a library's signed bytes when its code directory hash
                        is the build before's
                        bases: the channel's build before, main's, the other listed builds,
                        the newest three that left the list
                        publish, not-published, channels: from FrameFairy
                        sign: a second signature, over the time the build was published
cmd/speakertrail-app    install.go: keeps the build it replaces, opens the new one, and puts
                        the old one back when the new one does not say it started within a
                        minute. Relaunch goes through it too, not Wails' helper
internal/updates        refuses, unasked, a build of the channel followed older than the one
                        running
Wails' pkg/updater      downloads, checks the signature, unpacks, as before
```

Each layer uses only the ones under it, as in the record before.

What a build that does not start looks like: the new build writes a mark
into the app's folder once the window has loaded the interface and the
workspace answered. The old build's step waits up to a minute for it.
Without it, the step ends the new build, puts the old one back, opens
it, and writes down the SHA-256 of the build that failed. The app then
does not install that build by itself again, and Updates says it did not
start, with the build before running. A build put in on quit is opened
later, with nobody waiting. So each start first looks whether the start
before it, of the same build, said it started. When it did not, and the
build before is kept, it puts that one back the same way. Such a build
is undone after its second failed start.

## What follows

Estimates, read from the steps above, not measured:

| | today | after | change |
| --- | ---: | ---: | --- |
| publish job (s) | 273 | about 60 | about 78 % faster |
| push to Ready to update to, median (s) | 366 | about 150 | about 59 % faster |
| files a build adds to `dev` | about 69 | about 14 | about 80 % fewer |
| files on `dev` | 532 | about 120 | about 77 % fewer |
| a usual update (MB) | 0.12 to 0.51 | the same | 0 % |
| an app with only the libraries of an earlier build (MB) | 16.9 | 4.7 | 72 % smaller |

The pull requests that build it measure each line as these were
measured, from GitHub's record of 25 builds.

- It costs: about 11 MB downloaded by every publish to compare the
  libraries, a mark the app writes on each start, a step on quit that
  waits a minute where it waited seconds, and a second signature in
  each entry. The time goes beside the claim, not into it, so the apps
  from before still check the claim they know. An entry without the
  time is not installed by itself any more, only picked.
- It buys: a build on Updates about three and a half minutes sooner,
  every 25 builds about 90 minutes, a release that stays far from its
  limit without a cap doing the work, a broken build undone without a
  terminal, and no way to hand the app an older build of its channel.
- It was wrong if a build that changed no library still uploads a
  library's piece, if the publish job still takes over two minutes, or
  if a build that started fine is put back.

### For FrameFairy

FrameFairy could take the patches, the lean programs and the build on a
pull request's opening from here, and the undoing and the claim's time
from this design. That is FrameFairy's to decide, and a briefing for its
sessions once Tim says so.

### Issues, once Tim says go

In this order, the first the largest gain:

1. #128, signed bytes that change only with the code, as in simulation 2.
   Size medium.
2. The publish in Go, `publish`, `not-published` and `channels` from
   FrameFairy, part of #57. Size medium.
3. The release tool built once on main and attested, from FrameFairy,
   for #55. Size small.
4. The bases: the channel's build before, main's, the listed ones and
   the newest three that left. Size small.
5. A build that does not start is undone, the step on quit and Relaunch.
   Size medium.
6. Each entry signs when its build was published, and the app does not
   step back within its channel by itself. Size small.
