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
window, never in the other profile's. What is still to check is the same on
Tim's Mac, with `scripts/open-in-profile.sh`.

Two things to get right in the app:

- An unknown folder name makes Chrome create a new, logged out profile without
  a word. The app only uses folders it read from `Local State`, and says so
  when the chosen one has gone.
- The link is untrusted. A link that starts with `--` would reach Chrome as a
  command line switch. The app only passes links that start with `https://`
  and contain no spaces.

The app is not sandboxed, it is not sold through the App Store, so it may
start other apps with arguments.

## What the app will do

In Settings, Tim picks a browser and one of its profiles once, from a list the
app read from the browser's own files. Every LinkedIn button in the app then
opens its link there, in the background or in front.

## Try it on the Mac

```sh
bash scripts/open-in-profile.sh
bash scripts/open-in-profile.sh "Google Chrome" "<profile name>" https://www.linkedin.com/feed/
```

The first lists the browsers and profiles on the Mac. The second opens the
LinkedIn feed in the named profile. The feed shows Tim's own photo top left
when it landed in the right place.
