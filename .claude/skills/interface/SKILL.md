---
name: interface
description: How to change the Speaker Trail interface when you cannot see it. Use for any work in web/, a screen, a control, a colour, and above all when Tim reports that something looks wrong in the app. Covers the mock harness, what to measure, and how to prove a fix rather than claim one.
---

# Changing the interface without seeing it

A cloud session has no screen, and the app's window cannot open there. Tim
is the only one who looks at the app, on his Mac. Without care a session guesses, reports a fix, and Tim
finds the same bug still there. That costs him a round of testing each time.
These lessons come from Frame Fairy, where it happened more than once.

## The harness

The interface runs on its own against invented data:

```
cd web && MOCK=1 npx vite --port 5199 &
```

The mock in `web/mock/` answers the API as `docs/api.md` describes, with
invented people, and no login, like the app. Open it with Playwright,
which is installed globally, and Chromium from `/opt/pw-browsers/chromium`.
Never run `playwright install`. To see how WebKit draws something, run
WebKitGTK's MiniBrowser under `xvfb-run`.

To see a change against the real backend instead, serve `internal/api` over
HTTP from a small program in the scratchpad, with the interface from
`web/dist/`, a store in a temporary folder via `store.Open`, and a fake
finder that returns invented profiles, the way `internal/api/api_test.go`
does. Never real names, not even in a scratch file.

What has to keep working is a walk in `web/walks/`, run by `make walks`
and by CI: it starts the mock itself, drives Chromium, plays what macOS
and WebKit hand the app, and checks what the interface did, never waiting
on the clock. The sidebar's walks are ported from FrameFairy's
`frontend/preview/walks`. A walk is proven by making it fail on the code
before the fix.

Look at every change at 390 by 844, the narrowest window, and at 1280 by 900, in
light and in dark. Probes and screenshots go in the scratchpad, not in the
repository.

## Prove a fix, do not claim it

- **First make the bug happen.** A probe that shows the bug on the code
  before the change, then shows it gone after. A probe that passes on both
  proves nothing about the fix.
- **Measure the right thing.** A box's position says where it is, not what
  is painted there. A button's text says what it says, not whether it
  responds. Take the measurement that could only come out right if the bug
  is gone: the pixels in a screenshot, the scroll width of the page, the
  request the click sent, the answer on screen after it.
- **A page error is a result.** Attach `page.on("pageerror")` and
  `page.on("console")` in every probe. A thrown error in a loop can leave
  the page looking fine while nothing works.
- **When a screen looks wrong, compare it with a real app.** Tim's sense of
  wrong comes from the apps he uses every day. Check how a well-made app
  solves the same thing before inventing a solution.
- **A rule written in a comment is a claim to check.** Code that says what
  it guarantees may not guarantee it. Read what it actually does, and test
  it, before building on it.
- **The mock has to answer the way the app does.** Data made up by hand
  looks fine in a picture and hides the bug that only shows with the real
  shape. When the app's answer changes, change the mock with it, from
  `docs/api.md`.

## Work in hand

Tim wants to see at every moment what the engine does. Adopted from Frame
Fairy's rules, without its styling:

- **Say what happens now.** "Reading the profile from Exa", "Checking
  Yogalehrerin Köln". Not "Working". On the row of what is worked on,
  or in the `title` of the control that started it, never in a line of
  its own that pushes the page down (Tim, 22 November 2026, #43).
- **Show how far, where the work is.** The fill of `Busy.svelte`, with a
  band travelling over what is done: in the control that started the
  work, like Stop for a whole run, and across the row of each thing the
  work is on, like a person whose profile is read, several rows at once,
  the way FrameFairy fills a clip's card. A row holding it sets
  `position: relative`, `isolation: isolate` and `--beam-edge`, and its
  words read over the fill, `walks/look.mjs`. Work that cannot say how
  far it is shows the beam alone, never a fill stuck at a number it does
  not have. The fill never jumps back: count what the work will add, like
  links that join a run, before they are queued. A count says the same
  in figures, like New's 3 of 12 while a run goes.
- **Show time so far and about how long is left.** Time left is measured
  from how long the same kind of work took before, like the app's
  `seconds_left`, never guessed. It moves in steps of five seconds and in
  whole minutes from a minute on, and it counts down between answers.
  Until there is anything to measure against, it says so.
- **Work out of sight pulses.** `.dot.busy` on the place that leads to it,
  like People on the sidebar's rail.
- **One watch, one state.** The going run comes from `runWatch` in
  `lib/run.svelte.ts`, for every screen and the dot alike, so no screen
  shows a state another has left. A screen that shows the run calls
  `runWatch.watch()` in an effect. Start and Stop go through it and show
  at once, before the app answers. A run that ended says what it
  brought in a toast, from `runWatch` too.
- **The beam runs on the line round the control**, not a pixel inside
  it, with the fill and the motes inside that line. A control holding a
  beam says how far out its line lies in `--beam-edge`, which is
  `--line-w` for a `.btn`, and never clips with `overflow`, since that
  cuts the light off at the inside of its border. `walks/beam.mjs`
  measures every control that carries one. Tim saw the beam a pixel
  inside the border testing #24, issue #29, and FrameFairy fixed the same
  in its #795.
- **It can be stopped**, in the same place it was started.
- **One component per kind of work.** Work in hand shows itself through
  `Busy.svelte`, on a control and on a row alike. A new kind of work gets
  the same beam, fill and pulse, never a new animation.

Check it in the mock: the mock's run checks two searches a second and
reads profiles four at a time, about 4 s each, like the app's.
`walks/links.mjs` adds people from links and watches the rows fill and
New count. Probe the fill width and the time left over the whole run and
see that the fill only grows and the time left only shrinks.

## The look

Tim picked Morning Mist on 14 November 2026, with Dusk Fog as its dark
mode, from the mockups on https://claude.ai/artifact/U93dDSXobhvZGMmdYAPHec.

- **Flat, in solid colours.** Tim asked for it on 21 November 2026,
  issue #45: no shadows, no frosted glass, no mist behind the window and
  no bright edge on what is chosen. He disliked the 3D look of the chosen
  page in the sidebar. A pane, a list, a panel and a control lie on the
  page in a solid `--surface` or `--side`, set apart by colour and a line.
  A layer over the page, a list to pick from, a bubble, a dialog, a sheet,
  has a `--line-strong` edge instead of a shadow. A toast needs none: it
  is the ink with the page's colour for words, the other way round from
  the page under it. Where a line must take no room it is an inset
  box-shadow one pixel wide with no blur in `--line` or `--line-strong`,
  which the walk counts as a line. The light of work in hand, the beam,
  glows on purpose and is the one exception. `walks/look.mjs` looks for a
  shadow, a blur or a filter on People, Searches, Settings, Updates and
  Report, with a person open, in a sheet, a list open, a toast, an info
  bubble, the question before removing and a download running.
- **Every control takes its states from the same tokens.** Under the
  pointer `--hover`, pressed `--press`, both a little of the ink over what
  is there, `--hover-mix` and `--press-mix` of it, or `--surface-hover`
  and `--surface-press` for a control on `--surface`, so the two look
  alike side by side. A control on another ground mixes the same two
  shares into it. The primary button moves toward the ink, `--accent-hover`
  and `--accent-press`, darker in light and lighter in dark. A control in
  the colour of what it says, a toggle that is on or a button that
  removes, takes `--tone-hover` and `--tone-press`, a wash of its own
  words. A field's edge under the pointer is `--line-hover`. A button that
  cannot be pressed is `--disabled` with `--disabled-ink`, the same grey
  in every kind, never a paler copy of itself, except one with the beam,
  which works at full strength. A new control uses these tokens and never
  a number of its own. The one exception is the action on a toast, which
  greys out as its own words at less strength, since the grey would not
  read on the toast's dark ground.
- **What is chosen is one flat tint.** The page in the sidebar, the
  filter shown, the theme chosen, a pressed chip and the open person all
  wear `--chosen`, deeper under the pointer and pressed, with their words
  in `--chosen-ink`, the accent a shade toward the ink so they read at
  4.5 to 1 in every state. A list to pick from marks its choice the way
  a Mac menu does, with a tick and the words in `--chosen-ink`, since a
  tint there would read as the row under the pointer.
- **A button in focus wears a ring, a field its edge.** A button, a
  chip or a switch reached with the keyboard wears FrameFairy's ring,
  2 px wide and 1 px out from its edge, in the button's own colour,
  never one colour for all: `currentColor`, the colour of its words, so
  a plain button's ring is its ink, one that removes is red and a toggle
  that is on is green. A primary button and a switch that is on set
  `--ring` to the accent they are filled with, and a button greyed out
  with `aria-disabled` wears its grey. Tim asked for it testing #46 on
  21 November 2026, twice: Clear and the plain buttons beside Report
  wore Report's accent. A list to pick from wears the same ring 1 px
  inside its edge, as in FrameFairy, and none while it is open. A field
  in use keeps the grey edge it has under the pointer, `--line-hover`,
  and nothing more, since the caret says where typing goes. Tim found
  the accent edge lighting up only once the pointer left a field he
  was typing in, and asked for it gone from every field on 21 November
  2026. A row, a
  page in the sidebar and a segment of a choice show their own colour
  2 px inside their edge with a faint wash, since a ring would be cut
  off by the list or pane round them. The wash alone was too faint to
  find, Tim said on 17 November 2026. `walks/look.mjs` checks every
  ring's colour against its button's, and `walks/focus.mjs` measures
  that the ring or the edge changes by 3 to 1 in both themes, see
  `:focus-visible` in `app.css`.
  What is selected already, like the open person's row, shows nothing
  more.
- **One inset round a page and between its sections.** The space over
  the first control, at the sides, under the last section, and between
  the controls and the list or the next section is `--gap-page`, which
  is `--pad`, on every page, in a pane and in a sheet. More room under
  the last section scrolls a page whose content fits, which Tim found
  on the person on 22 November 2026, issue #77, and `walks/person.mjs`
  measures it.
  The person beside the list mirrors it: the name and the cross on the
  row of the search field, the person's buttons on the row of the filter,
  and the first section on the line the list starts on. Tim marked each
  place it was off on a screenshot testing #60 on 21 November 2026, and
  `walks/look.mjs` measures them at 1280 and 390 px.
- **Every list looks the same.** People, Searches and Settings read in
  one centred column, `--col-w`, with the controls above a `.list` of
  rows, each a name over one line of what is known, with what is
  connected in green at its start. Tim asked for one look for all lists
  on 14 November 2026.
- **A control is as wide as what it shows now.** A button fits what it
  says this moment, never a width guessed in `em` and never room kept
  for words it may say later: Check grows to Checking while it checks,
  as in FrameFairy. A list to pick from is as wide as what is picked, up
  to `--pick-max`, and cuts the rest with an ellipsis. At 390 px the
  controls that share a line stay as wide as their words too. Tim found
  Run, Add, Check and the LinkedIn profile list too wide twice testing
  #46, and `walks/look.mjs` adds up what each button and list on every
  page shows, at 1280 and 390 px, and fails one that is wider.
- **Two gaps between controls, and no others.** Two controls side by
  side or one over the other, the buttons of a row, a field and its
  button, the search, the order and Run, stand `--gap-control` apart.
  The items that share one ground, the segments of a choice and the
  pages in the sidebar, stand `--gap-inset` apart and as far in from
  their ground's edge, so the one under the pointer never touches the
  one chosen. Tim found them touching, and buttons 6, 8 or 10 px apart,
  testing #46, issue #63. Two rows of controls, like the search, the
  order and Run over the filter on People, stand `--gap-control` apart
  too. `walks/look.mjs` measures every two neighbouring controls and
  rows of controls on People, Searches with its exclusions open,
  Settings with the question before removing, Updates and Report, at
  1280 and 390 px. A person open is not measured yet. The sidebar's
  4 px differs from FrameFairy's 2 px on purpose.
- **Tab goes the way the page is written.** macOS only tabs between
  buttons when full keyboard access is on, so a window, a sheet and a
  question move the focus themselves, all through `moveTab` in
  `lib/keys.ts`, never through a list of their own. Where Tab should
  take another way than the eye, the page is written in Tab's order and
  drawn in the other with CSS: Report a Problem's row is written Report,
  Clear, the info mark, Attach, Dictate and drawn with `row-reverse`, so
  Enter, Tab, Enter ends a dictation and makes the report (Tim, testing
  #46). An info mark is help, not a control, and Tab passes it by with
  `tabindex="-1"`, as Tim asked. A button that cannot be pressed now, in a row Tab walks, is
  greyed out with `aria-disabled` and keeps its place, and its handler
  does nothing. One with `disabled` drops out of the order, and Tim
  found the order changing with an empty field.
- **There is one list to pick from, `Pick.svelte`.** It is FrameFairy's,
  built on bits-ui, so a list looks the same in the window on a Mac as it
  does in the harness, and a row can say more than a name: a size, a
  group, whether a model is downloaded. Never a native `<select>`, which
  the Mac draws in its own box. The People order, the LinkedIn profile,
  the Updates channel and the language model all use it, and
  `walks/pick.mjs` walks it.
- **The sidebar wears a dot where something needs a look.** Updates, from
  where its updates stand, and Settings, from the Exa key and the
  language model's `standing`: red when runs or drafts cannot work, amber
  when a download would do, pulsing while the model comes. The app judges
  it, the window only draws it. Tim asked for the one on Settings on 21
  November 2026. `walks/draft.mjs` and `walks/pick.mjs` walk its states.
- **Opening something never relays out what is beside it.** A row has the
  same lines at any width, so a person opening only narrows the list. A
  container query on a pane that another pane squeezes brings the shuffle
  back. `web/walks/person.mjs` checks it.
- **Every colour has a light and a dark value.** Check both, and check that
  small text keeps a contrast of at least 4.5 to 1 on `--bg` and `--surface`,
  in every state. `walks/look.mjs` measures the words of the controls at
  rest, under the pointer and pressed, that a run's fill stands out from
  its track by 3 to 1, and that the band travelling over it, `--sheen`,
  shows on the fill by 1.5 to 1 and stays on it.

## What to report

What changed, the screenshots before and after, the measurement that proves
it, and exactly what Tim should look at in the app and what he should see.
