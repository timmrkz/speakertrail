# Working with Claude through GitHub

The code lives on GitHub. Claude works on it in cloud sessions at
[claude.ai/code](https://claude.ai/code) and delivers every change as a pull
request. Your Mac only pulls and runs.

```
you describe a task at claude.ai/code
  → Claude works on a branch in the cloud, runs make test, pushes
  → a pull request opens, CI tests it and builds the image
  → you review and merge
  → the merge deploys to Scaleway by itself
  → on your Mac, to try it locally: git pull && make run
```

## The cloud environment

At claude.ai/code, the environment for this repository has:

| Field | Value |
| --- | --- |
| Network access | a custom allowlist with the event sites, so a session can save real pages as test fixtures |
| Setup script | the full content of [`scripts/cloud-setup.sh`](../scripts/cloud-setup.sh) |

The setup script installs Go 1.27 and prepares the local Postgres. A session
starts Postgres by itself, through `scripts/session-start.sh`.

The allowlist only affects the cloud sessions. The deployed app and your Mac
reach every site. When the engine finds sources on new domains, they are not
on the list yet: add them in the environment settings when a session needs
to look at them. A change there applies to new sessions.

## Everyday loop

1. **Start a session** at claude.ai/code with the `speakertrail` repository
   and its environment. Describe the task the way you would in chat,
   screenshots included.
2. **Claude works** on a branch, opens the pull request with its first
   commit and keeps pushing to it. It reads `CLAUDE.md` first.
3. **CI** tests the pull request and builds the Docker image. A red check is
   Claude's to fix.
4. **Review** on GitHub or in the session. Merge when it is right.
5. **The merge deploys.** GitHub Actions builds the image and updates the
   app and the nightly job on Scaleway, see
   [deploy/scaleway/README.md](../deploy/scaleway/README.md).
6. **To try it on your Mac:**
   ```
   git checkout main
   git pull
   make run
   ```

To try a pull request before merging:

```
git fetch origin
git checkout BRANCH-NAME
make run
```

## What stays on your Mac

- Looking at the interface for real, although the phone works for that too
  once it is deployed.
- A crawl of the real sites against a local database, `make crawl`.
- A copy of production data, `make pull-prod`, see
  [BUILD.md](BUILD.md#production-data-on-this-machine).

## Chat or cloud session

Ideas, planning and questions work well in a normal claude.ai chat. Anything
that changes the code belongs in a cloud session, so it arrives as a pull
request.
