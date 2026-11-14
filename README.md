# Speaker Trail

Speaker Trail finds people in North Rhine-Westphalia who run their own
business and could be guests on the podcast *My First Memory*. It is a Mac
app: one window, with People, Searches, Settings and Updates, and
everything it knows in one plain file on the Mac. It lives in
`/Applications` and updates itself to main or to any open pull request,
see [docs/UPDATES.md](docs/UPDATES.md). No Docker, no database, no server.

```sh
make run
```

builds the app and opens it. [docs/BUILD.md](docs/BUILD.md) has the rest:
the first start, where things are kept, and how to bring over the people
connected in the Docker version.

## How it works

A run asks **profile searches**, like "Yogalehrerin Köln". Each one asks Exa
once for LinkedIn profiles from its own index, 4 side by side. Only people
who live in NRW, were active on LinkedIn in the last six months and run
their own business are kept. Consultants, agencies, corporate titles and
whatever Tim's exclusions name, like "Online Marketing", are left out.
People are scored by the fit rubric from
[docs/search-strategy.md](docs/search-strategy.md): signals for and against,
each with the passage that shows it.

A search that finds new people stays active. One that finds nobody new
moves from candidate to probation and retires, and a search is asked again
after 30 days. A run takes 10 due searches, those that led to the most
people Tim connected with first, so it stays short enough to watch, and
can be stopped. Tim marks a person as connected once he connected with
them on LinkedIn, and the rest is simply open. Exa gets a monthly budget in Settings,
and runs stop asking once it is spent.

The engine never requests LinkedIn itself. Profiles come from Exa's index.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/speakertrail-app` | The app: the window, its updates, and `carry-over` |
| `cmd/speakertrail-release` | Makes the update key, signs builds, writes the channel list |
| `internal/store` | The file: people, searches, runs, settings, the monthly budget |
| `internal/engine` | Runs: which searches are due, asking them side by side, stopping |
| `internal/api` | The JSON API the interface talks to, described in [docs/api.md](docs/api.md) |
| `internal/keys` | The Exa key in the macOS keychain |
| `internal/updates` | Where builds to update to come from, and checking their signatures |
| `internal/search` | Exa's index of LinkedIn profiles |
| `internal/place` | Whether a place lies in NRW, and which city it names |
| `internal/rubric` | The fit rubric: signals for and against a person, each with its passage |
| `web` | The interface, Svelte 5 with TypeScript |
| `scripts` | Installing what is missing, bundling the app, the carry-over, the cloud session setup |
| `docs` | Building, the API, the search strategy, working with Claude |

The public calendar, the crawler of event pages, post searches and the
server that ran in Docker and on Scaleway were removed when Speaker Trail
became a Mac app. They are in git history, before pull request #9.

## Rules the code keeps

- No request ever goes to linkedin.com or instagram.com.
- No email addresses or phone numbers are stored.
- The file with real names stays on the Mac and never goes into the
  repository. Tests and mock data use invented people.
- The Exa key lives in the keychain, never in a file.
