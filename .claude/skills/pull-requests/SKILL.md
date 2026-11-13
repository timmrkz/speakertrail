---
name: pull-requests
description: How a pull request is made in any project, from the first push to the merge. Open it first and push to it as the work goes, one small self-contained change per pull request, described the way the template lays it out, watched through its events, reviewed once by an agent that did none of the work, and called ready only when it is reviewed, mergeable and green. Read it before branching, before the first push, before describing a pull request, and before saying one is ready.
---

# Pull requests

A pull request is how work reaches the user. They follow it while it
happens, try it, and merge it. Work with no pull request is work that
has not been handed over.

## 1. Open it first, not last

- **Open it with the first push,** without being asked and without
  asking, and then keep pushing to it. That is where the user follows
  the work. A pull request opened late is the same mistake as no pull
  request at all.
- **Push real work early, not placeholders.** The first push carries the
  first piece of the change, a failing test or the first working part,
  never a plan or a note alone. A push starts CI, so push when there is
  something for CI to check, and do not hold finished work back either.
- **Push once the tests of what changed pass,** and let CI run the rest.
  Before the push, run the test written for the change and the tests of
  the part it is in: seconds. Nothing more runs in the session. CI runs
  the whole of what the change reaches, on the machines sized for it.
  Only a commit known to be broken, a change half made, is held back,
  and the reply says it is not ready to try.
- **Right after opening it, check that it merges into main,** because
  main may have moved while the branch was being made.

## 2. One self-contained change

The way Google's
[code review guide](https://google.github.io/eng-practices/review/developer/small-cls.html)
draws it, a pull request is the right size when:

- It makes the smallest change that addresses just one thing, usually
  one part of a feature rather than the whole feature.
- It brings the tests for what it changes.
- Everything a reviewer needs to understand it is in the pull request,
  its description, the code as it is, or a pull request already
  reviewed.
- The product keeps working for the people using it, and for the
  sessions working on it, once it is merged.
- It is not so small that what it means is hard to see.

When in doubt, smaller. About 100 changed lines is a usual size and 1000
too many, and a change spread over many files counts as bigger. The
review agent and the user may ask for a pull request to be split for its
size alone.

- **A refactoring goes in a pull request of its own,** before the change
  that needs it. A small one, and small cleanups like a name, a doc line
  or a rule the change made wrong, go in with the change.
- **Work that builds on an open pull request takes its branch as its
  base,** so its diff shows only its own change, or it waits until the
  first is merged. The same commits are never pushed into two pull
  requests.
- **One session, one pull request at a time.** A session opens the next
  only once its open one is merged or closed. Two at once split its
  attention, and the user cannot tell which a reply is about. Reading
  another pull request is not working on it.
- **At most five pull requests open at a time** on the whole repository,
  from every session together. Count them before opening one. When five
  are open, open none: say so to the user and wait. The user can lift
  the limit for one pull request by saying so.

## 3. Described the way the template lays it out

`.github/pull_request_template.md` is the one place that says how a pull
request is described. Short and concrete at the top, as long as it needs
in the proof.

- **The opening names the issue** it works on, and says `Fixes #N` for
  each issue it finishes.
- **Root cause and The fix** name the files and the names in the code.
- **Proof** says which tests fail on main and pass here, the numbers
  before and after, and whether CI passed on the last commit.
- **How to test is what the user reads first,** so it is never left out.
  Each step is one they can take as it stands. A short Before and After
  is no replacement for it.
- **Found along the way** lists each thing logged as an issue, or says
  Nothing.
- **The description says what the pull request is now.** When the work
  changes, the description changes with it.

## 4. Watched, quietly

- **Subscribe to its events the moment it is opened** or taken over, so
  everything that needs the session reaches it by itself: a CI run that
  fails or passes, a review, a comment, and a conflict with main.
- **Act on each event at once.** A failing CI run is still your work, so
  is a conflict, so is a review comment.
- **Never poll, never set up a recurring check,** and never write a
  message that says nothing happened. The user comes back when they are
  ready.

## 5. Main moving

- **Merge main in for a conflict, and only then.** Main moving is no
  reason to merge it into a pull request: the merge adds a commit and a
  CI run, and changes nothing being tested.
- **When main no longer merges,** the session merges main in at once,
  without asking, resolves the conflict, reads `CLAUDE.md` and the
  skills again, because main may have changed them, runs the tests of
  what the resolution touches, and pushes. GitHub runs no CI on
  a pull request in conflict with main, so one is never left that way
  while it waits for the user.

## 6. Reviewed once, by an agent that did none of the work

- **When all the work is done** and the tests of the change pass, the
  session asks a new agent to review the pull request, the way the review
  skill says. It comes once, not after every push. A review after later
  rounds is the user's to ask for.
- **Its brief carries sources, not conclusions:** the user's own
  messages, the issue, the product's goals, and the path of
  `docs/ARCHITECTURE.md`, never the session's summary of them. For a
  change to the architecture it carries the decision record too, with its
  candidates.
- **The review is a file outside the checkout,** never committed and
  never posted on GitHub, where it would read as the user's instruction.
- **The session posts it in its reply:** the file, and a short list of
  the findings by number.
- **Each finding is then fixed, or answered with why it is wrong,** and
  reported by its number: what changed, or why nothing did. "Addressed
  the review" is no report. Only a finding outside the pull request
  becomes an issue. One that needs the user's choice goes in their list.

## 7. Ready means reviewed, mergeable and green

Before a reply says a pull request is ready to try or to merge:

1. The agent's review is back, and every finding is handled.
2. GitHub says it merges into main.
3. CI passed on its last commit. A green check from before a conflict
   says nothing about the commit there now.

## Before saying it is ready

1. It is one self-contained change of a reviewable size.
2. The description follows the template, How to test included.
3. It is subscribed to, and every event so far was acted on.
4. The review is back, and every finding is reported by its number.
5. It merges, and CI passed on the last commit.
