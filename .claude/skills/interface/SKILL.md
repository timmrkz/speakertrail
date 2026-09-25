---
name: interface
description: How to change the Speaker Trail interface when you cannot see it. Use for any work in web/, a screen, a control, a colour, the calendar, and above all when Tim reports that something looks wrong on his phone. Covers the mock harness, what to measure, and how to prove a fix rather than claim one.
---

# Changing the interface without seeing it

A cloud session has no screen. Tim is the only one who looks at the app,
mostly on his phone. Without care a session guesses, reports a fix, and Tim
finds the same bug still there. That costs him a round of testing each time.
These lessons come from Frame Fairy, where it happened more than once.

## The harness

The interface runs on its own against invented data:

```
cd web && MOCK=1 npx vite --port 5199 &
```

The mock in `web/mock/` answers the API as `docs/api.md` describes, with
invented people. The password is `speakertrail`. Open it with Playwright,
which is installed globally, and Chromium from `/opt/pw-browsers/chromium`.
Never run `playwright install`.

To see a change against the real backend instead, build it into the app and
run the app, `make run`, with invented data put in through the resolver the
way `internal/server/server_test.go` does. Never real names, not even in a
scratch database.

Look at every change at 390 by 844, Tim's phone, and at 1280 by 900, in
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
- **The mock has to answer the way the server does.** Data made up by hand
  looks fine in a picture and hides the bug that only shows with the real
  shape. When the server's answer changes, change the mock with it, from
  `docs/api.md`.

## What to report

What changed, the screenshots before and after, the measurement that proves
it, and exactly what Tim should look at on his phone and what he should see.
