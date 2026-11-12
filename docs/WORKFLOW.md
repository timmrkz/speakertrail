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

## What is on your Mac

Git, this folder, and what make installs with Homebrew the first time, Go
and Node.js. See [BUILD.md](BUILD.md).

## Chat or cloud session

Ideas, planning and questions work well in a normal claude.ai chat. Anything
that changes the code belongs in a cloud session, so it arrives as a pull
request.
