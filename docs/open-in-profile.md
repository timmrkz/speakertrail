# Opening LinkedIn in the right browser profile

The Mac app has to open a person's LinkedIn profile where Tim is logged in to
LinkedIn. He has several browser profiles, each with its own windows, and only
one of them holds his LinkedIn login. There is no LinkedIn API, so this is the
only way the app reaches LinkedIn.

## The answer: yes, it works

The app does not need to find the right **window**. It needs the right
**profile**. A login lives in the profile's cookies, and every window of that
profile shares them. So the app names the profile, and the browser picks one
of that profile's windows, or opens one.

How well a browser lets another app name a profile:

| Browser | How the app names the profile | Sure? |
| --- | --- | --- |
| Chrome, Brave, Edge, Vivaldi | `open -n -a "Google Chrome" --args --profile-directory="Profile 1" <link>` | Yes. Proven below, and the way Finicky has done it for years |
| Safari | It cannot. Safari's own setting does it instead: Settings, Websites, Open Links With Profile, linkedin.com set to Tim's profile. The app just hands Safari the link | Yes, set once in Safari. It does not apply while LinkedIn is open in another Safari profile |
| Arc | AppleScript: `tell front window to tell space "Work" to make new tab` | Likely. Spaces belong to profiles. Arc gets no new features since its makers moved to Dia |
| Dia | AppleScript can list and focus windows and tabs, no command to open a link in a profile is documented | Not yet |
| Firefox | `open -n -a Firefox --args -P "name" <link>` | Shaky. Before Firefox 137, a link from another app opened in whichever profile ran. Since then it goes to the last used profile, not yet tested with a named one |

Plain `open <link>`, which is what clicking a link does today, goes to the
default browser and lands in whichever profile was used last. That is the coin
toss to avoid.

## How Chrome does it

Chrome keeps its profiles in `~/Library/Application Support/Google/Chrome/Local State`,
under `profile.info_cache`. Each entry is a folder name, like `Default` or
`Profile 1`, with the name Tim gave it. The app reads that file, shows Tim the
names, and keeps the folder he picks.

To open a link, the app starts a second copy of Chrome with `open -n` and the
profile's folder. That copy finds the Chrome already running, hands it the
link and quits. The running Chrome then:

- opens the link as a new tab in that profile's last used window, when the
  profile has a window open,
- opens a new window for that profile, when it has none,
- starts in that profile, when Chrome is not running.

All three were tested in a cloud session on 12 Nov 2026 with Chromium 153 on
Linux, which uses the same hand over as Chrome on a Mac. A small local web
server played LinkedIn. Each profile logged in as someone else, and every link
sent to `Profile 1` arrived with `Profile 1`'s login cookie, as a tab in its
window, never in the other profile's.

On 12 Nov 2026 Tim ran `scripts/open-in-profile.sh` on his Mac, macOS with
Google Chrome and two profiles. It listed both, and a link sent to the second
profile, the one logged in to LinkedIn, opened there, logged in. This is the
way the app does it.

Two things to get right in the app:

- An unknown folder name makes Chrome create a new, logged out profile without
  a word. The app only uses folders it read from `Local State`, and says so
  when the chosen one has gone.
- The link is untrusted. A link that starts with `--` would reach Chrome as a
  command line switch. The app only passes links that start with `https://`
  and contain no spaces.

The app is not sandboxed, it is not sold through the App Store, so it may
start other apps with arguments.

## How the app will do it

This is the plan for the Mac app. Follow it when building the LinkedIn button.

### Settings

- Settings has one entry, **LinkedIn opens in**, a menu of Chrome's profiles by
  the names Tim gave them, read from `Local State` each time Settings opens.
- The app keeps the profile's **folder**, like `Profile 1`, not its name.
  Names can change, folders do not.
- With only one profile, it is chosen without asking.
- If the kept folder is gone from `Local State`, the setting shows that and
  asks for a new choice.

### The Go side

A small package, `internal/browser`, with no knowledge of the interface:

- `Profiles(app string) ([]Profile, error)` reads
  `~/Library/Application Support/<folder>/Local State` and returns each
  profile's folder and name. The folder for each browser is the table in
  `scripts/open-in-profile.sh`, Chrome first.
- `CheckLink(link string) error` passes only a link that parses with
  `net/url`, uses `https`, and has no spaces. A crawled link is untrusted, see
  the warning above.
- `Command(app, folder, link string, background bool) []string` builds the
  arguments: `-n`, `-a`, the app, `-g` when in the background, `--args`,
  `--profile-directory=<folder>`, the link. A pure function, so tests can
  check every argument.
- `Open(ctx, app, folder, link string, background bool) error` checks the
  link, checks the folder against `Profiles` again, then runs
  `exec.CommandContext(ctx, "open", Command(...)...)`. Never through a shell.

Never plain `open <link>`, and never AppleScript window lookups. Chrome's
AppleScript does not say which profile a window belongs to.

### Tests

- `Profiles` against a `Local State` in `testdata/`, with invented profile
  names, including a broken file and one without profiles.
- `CheckLink` as a table: a LinkedIn link passes, `--remote-debugging-port=1`,
  `http://`, `javascript:`, `file://` and a link with a space fail.
- `Command` as a table, foreground and background.
- Nothing in the tests starts a browser or touches the network.

### The interface

- The LinkedIn button in the person's detail opens their profile link with
  `Open`. A click shows at once: the button says it is opening until `open`
  returns, which takes well under a second.
- When no profile is chosen, the button does not guess. It says so and leads
  to Settings, because guessing is the coin toss this whole page avoids.
- An error from `Open` shows beside the button in plain words, like "Chrome
  has no profile Profile 1 any more".

## Try it on the Mac

```sh
bash scripts/open-in-profile.sh
bash scripts/open-in-profile.sh "Google Chrome" "<profile name>" https://www.linkedin.com/feed/
```

The first lists the browsers and profiles on the Mac. The second opens the
LinkedIn feed in the named profile. The feed shows Tim's own photo top left
when it landed in the right place.
