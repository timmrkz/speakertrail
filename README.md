# Speaker Trail

Speaker Trail finds people in North Rhine-Westphalia who run their own
business and could be guests on the podcast *My First Memory*. It is a Mac
app: one window, with People, Searches and Settings, and everything it
knows in one plain file on the Mac. No Docker, no database, no server.

```sh
make run
```

builds the app and opens it. [docs/BUILD.md](docs/BUILD.md) has the rest:
the first start, where things are kept, and how to bring over the people
kept in the Docker version.

## How it works

A run asks **profile searches**, like "Yogalehrerin Köln". Each one asks Exa
once for LinkedIn profiles from its own index, 4 side by side. Only people
who live in NRW, were active on LinkedIn in the last six months and run
their own business are kept. Consultants, agencies and corporate titles are
left out. People are scored by the fit rubric from
[docs/search-strategy.md](docs/search-strategy.md): signals for and against,
each with the passage that shows it.

A search that finds new people stays active. One that finds nobody new
moves from candidate to probation and retires, and a search is asked again
after 30 days. A run takes the 10 most due searches, so it stays short
enough to watch, and can be stopped. Exa gets a monthly budget in Settings,
and runs stop asking once it is spent.

The engine never requests LinkedIn itself. Profiles come from Exa's index.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/speakertrail-app` | The app: the window, and `carry-over` |
| `internal/store` | The file: people, searches, runs, settings, the monthly budget |
| `internal/engine` | Runs: which searches are due, asking them side by side, stopping |
| `internal/api` | The JSON API the interface talks to, described in [docs/api.md](docs/api.md) |
| `internal/keys` | The Exa key in the macOS keychain |
| `internal/search` | Exa |
| `internal/rubric` | The fit rubric: signals for and against a person, each with its passage |
| `web` | The interface, Svelte 5 with TypeScript |
| `scripts` | Installing what is missing, bundling the app, the carry-over |
| `docs` | Building, the API, the search strategy, working with Claude |

## Paused

The calendar, the crawler of event pages, post searches and the server
that ran in Docker are paused. Their code is still here, in
`cmd/speakertrail`, `internal/server`, `internal/pipeline`, `internal/queue`,
`internal/db` and the rest, with `compose.yaml`, the `Dockerfile` and
`deploy/`. The app does not use any of it. Their API is in
[docs/server-api.md](docs/server-api.md).

## Rules the code keeps

- No request ever goes to linkedin.com or instagram.com.
- No email addresses or phone numbers are stored.
- The file with real names stays on the Mac and never goes into the
  repository. Tests and mock data use invented people.
- The Exa key lives in the keychain, never in a file.
