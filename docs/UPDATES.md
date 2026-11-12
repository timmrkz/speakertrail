# Updates

Speaker Trail lives in `/Applications` and updates itself. Every push to
main and to an open pull request builds the app on GitHub. The app's
Updates page lists main and the open pull requests. Pick one, and the app
downloads its newest build by itself. **Relaunch** restarts the app into
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
   or ends, whatever branch was built. It works out the channel, commit
   and version again from GitHub's own record of the run, checks what the
   app links and asks for with `cmd/speakertrail-release sign`, signs the
   zip with the key from the environment `updates`, and uploads it with
   its entry to the pre-release `dev`.
3. `scripts/channel-list.sh` writes the channel list again from every
   entry, as `channels-a.json` and `channels-b.json` in `dev`. A pull
   request that closed leaves the list. The commit that was built gets
   the status **Ready to update to** on the pull request.

A build takes a few minutes after the push. A version reads like
`0.1.0-pr9.e808a87`: the app's version, the channel and the commit.

Until the pull request that adds this is merged, `publish.yml` is not on
main, so nothing is published yet.

## What the app does

`cmd/speakertrail-app/updates.go` and `internal/updates`:

- It reads the channel list when it starts, when a channel is picked and
  every ten minutes, every twenty seconds while a newer commit is being
  built.
- When the channel followed has a build that is not the one running, it
  downloads it into `~/Library/Caches/Speaker Trail/builds`, checks the
  signature against the public half built into it, and has it ready.
- **Relaunch** waits while a run goes, so a run is never stopped half
  way. The engine refuses a new run while the app relaunches.
- A pull request that closed shows as closed, and nothing downloads until
  another channel is picked.
- What it picked is in `updates.json`, in the app's folder beside
  `speakertrail.json`.

The interface asks `/api/updates`:

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

## What is never signed

`speakertrail-release sign` reads every program in the zip before it
signs, see `cmd/speakertrail-release/inputs.go`. It refuses a build that
links a library not on its list, or names what taps keys, records the
screen, opens the camera or asks for administrator rights. A refused build
says why on the commit, as a failed **Ready to update to**.
