# Working with Claude through GitHub

The code lives on GitHub. Claude works on it in cloud sessions at
[claude.ai/code](https://claude.ai/code) and delivers every change as a pull
request. The app on your Mac updates itself to it.

```
you describe a task at claude.ai/code
  → Claude works on a branch in the cloud, runs make test, pushes
  → a pull request opens, CI tests it, on Linux and on a Mac
  → GitHub builds the app for it, Ready to update to turns green
  → in the app, Updates: pick the pull request, then Relaunch
  → you review and merge
```

## The cloud environment

At claude.ai/code, the environment for this repository has:

| Field | Value |
| --- | --- |
| Network access | a custom allowlist, with api.exa.ai for live checks |
| Setup script | the full content of [`scripts/cloud-setup.sh`](../scripts/cloud-setup.sh) |

The setup script installs Go 1.27, and GTK 4 and WebKitGTK 6, which the
app's window compiles against on Linux. The window cannot open in a
session, but the app builds and every test runs.

## Everyday loop

1. **Start a session** at claude.ai/code with the `speakertrail` repository
   and its environment. Describe the task the way you would in chat,
   screenshots included.
2. **Claude works** on a branch, opens the pull request with its first
   commit and keeps pushing to it. It reads `CLAUDE.md` first.
3. **CI** tests the pull request and builds the app on an Apple silicon
   Mac. A red check is Claude's to fix. A few minutes after each push the
   status **Ready to update to** turns green.
4. **Try it** in the app: Updates, pick the pull request, and Relaunch
   once its build is downloaded. See [UPDATES.md](UPDATES.md).
5. **Review** on GitHub or in the session. Merge when it is right, and
   pick main again on Updates.

`make run` still builds a change on the Mac itself, from a checkout.

## When main moves

GitHub runs no CI on a pull request that conflicts with `main`, and
sends no event when one starts to conflict. So after every push to
`main`, [`conflicts.yml`](../.github/workflows/conflicts.yml) merges each
open pull request of this repository into it in memory, with `git
merge-tree`, and comments on each one that no longer merges, naming the
files. It tries a pull request after a push to its branch as well, at
the commit pushed, since that push may conflict. It tells a pull request
once for each pair of its commit and `main`'s, so every push to `main`
that still leaves it in conflict tells it again. A comment that gets
lost on its way to the session is followed by the next, and that costs
no run of its own. When several pushes to `main` come close together,
the newest run stops the older ones, and a hidden mark in the comment,
both commits, keeps a run stopped halfway from telling a pull request
twice about the same pair. The work is the command `conflicts` of
`speakertrail-release`, with its tests, and the workflow only calls it.
It is ported from FrameFairy, where the same steps are shell.

It comments, so it needs a token that writes, and it gets one only from
`main`'s own copy of the file, the way `publish.yml` does. A push to
`main` starts it, and a push to any other branch starts it through
`builds.yml`, which GitHub reports to `main`'s copy. A branch's own copy
never runs with the token, because a branch is code nobody has reviewed
yet. `TestNoPullRequestTarget` in `internal/workflows` fails on a
workflow that runs on `pull_request_target`, and on `conflicts.yml`
starting any other way.

The comment wakes the session that owns the pull request. It comes from
a bot, so the session reads it as news, checks on GitHub that its pull
request does not merge, and then merges `main` in at once, without
asking, resolves the conflict, reads `CLAUDE.md` again and pushes. A
pull request that conflicts from its first push is not open when that
push runs the workflow, so the session also checks right after opening
it, and before it says it is ready, with `git fetch origin main && git
merge-tree --write-tree --name-only origin/main HEAD`, which fails and
names the files when it does not merge. A pull request that still
merges is left alone: merging `main` into it would only add a commit, a
CI run and a build to take, and change nothing being tested.

## What is on your Mac

Git, this folder, and what make installs with Homebrew the first time, Go
and Node.js. See [BUILD.md](BUILD.md).

## Chat or cloud session

Ideas, planning and questions work well in a normal claude.ai chat. Anything
that changes the code belongs in a cloud session, so it arrives as a pull
request.
