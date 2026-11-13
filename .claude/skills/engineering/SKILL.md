---
name: engineering
description: What good engineering looks like in any project, the first principles every change follows. Think and verify before building, take away the cause and never only the symptom, one primitive per thing, untrusted input checked at the boundary, failures contained, one command to build, docs that say what the code does now, and numbers a person can compare. Read it before any change to code, build or CI, and before fixing anything.
---

# Engineering

Good engineering is the simplest change that makes the product better
for the people using it, made where it belongs, proven, and cheap to
keep. Everything below follows from that. The architecture skill says
where a change goes, the testing skill how it is proven, the review
skill how it is judged. This one says how it is made.

## 1. Think first

- **Research and check before answering.** Read the code and the docs
  you are about to change, before the change.
- **Verify facts instead of assuming them.** What a tool or a library
  does at the version used, what a browser supports, what a number is.
  Look it up at the source, the official docs or the code itself, and
  say where it came from.
- **Numbers from first principles.** Check that a cost makes sense
  before accepting it because the arithmetic adds up. Why does one small
  page take 2 MB? Why does loading take five seconds? Work out what it
  should be from what the product needs, and look for the difference.
- **Known problems have known answers.** A cache, a retry, an undo, an
  update flow, an animation curve: look up how established software does
  it before inventing. Where people already know a way of doing
  something, an icon, a word, a place for a control, do it their way.
- **Ask at most one question at a time**, and only when the answer
  changes what gets built. Everything else is decided, and the decision
  said.

## 2. Root causes only

- **Every fix takes the cause away.** A change that leaves the cause in
  place and only ends what it did this time, a place that gives up on
  it, a longer wait, a case skipped, is a workaround, and none is made.
  A workaround closes its issue and leaves the cause to come back under
  the next one, and nobody sees it is the same thing again.
- **Evidence comes first.** The user's report or log, the failure
  reproduced with real input on a machine people use, or the order of
  events made to happen on purpose. When the cause is not known yet,
  finding it is the work. A change that only helps find it, a log line
  or a trace, says so and closes nothing.
- **Whom it reaches decides where it is fixed.** What only a test's
  environment does, a CI runner or a stand-in, is fixed in how the test
  uses it, never in the product. What reaches a person is fixed once in
  our code, in one place, for every path it can come through.
- **A fix makes the design simpler, or it is the wrong fix.** Ask
  whether it streamlines the design or adds an exception that works
  around what is there. A fix that is generic and sound makes the next
  case of the same kind right by design.
- **Several symptoms after a run of merges often have one cause.** Look
  for it before fixing each, or the fixes stack up on each other.

## 3. One primitive per thing

- **A new use goes through what exists, extended rather than copied.**
  Before adding a second way to store, fetch, show or validate a thing,
  find the one there is and extend it. Where two uses differ, that is a
  field on the one thing, not a copy with its own rules.
- **A copy starts out the same and drifts,** and every place that
  handles the thing then needs to know which copy it has.
- **Something used in two places is one component, used, never
  copied.** A change to it is made once and is true everywhere.
- **Invalid states are made impossible, not handled.** Where the types
  or the shape of the data can rule a state out, they do.

## 4. At the edges

- **Input from outside is untrusted.** What a person typed, a file they
  imported, a request, an answer from another service. It is read by
  code made for it, at the boundary, and checked there, never parsed on
  the way. A reader of outside bytes has a fuzz target.
- **Secrets stay out of code, logs and the repository.**
- **Every path is judged by where it really leads,** inside the folder
  it is allowed, not by what it is called.
- **Text from outside is escaped where it is shown,** by the one helper
  that does it, never by hand in each place.

## 5. When things go wrong

- **A failure keeps its kind.** An error is returned to its caller with
  what went wrong and where, never swallowed, never turned into a
  default that hides a broken invariant.
- **A failure fails its own work, not the program.** A request or a job
  that goes wrong ends with its reason, and everything else carries on.
- **Logs make the product testable in the field.** Structured logs, from
  an established library, at the places a bug would be found, tagged
  with what they are about. One report from a person using the product
  should be enough to find the bug, without a second round.

## 6. Build and keep

- **One command, from a fresh checkout to a running product.** Nobody
  should have to remember a second one. The same command runs on a
  laptop, in a session and in CI.
- **A build finishes without warnings,** and CI fails on one. A warning
  lived with is a warning nobody reads.
- **Versions are pinned,** the language, the tools, every dependency and
  every action in a workflow, to an exact version or commit. Raising one
  is a line in a pull request, where it can be read first.
- **Few dependencies, each one earning its place.** A dependency is kept
  up for as long as it lives. Prefer the platform's own feature, then a
  small, widely used library, then code of our own.
- **Build output is never committed.**
- **Maintenance never ends, so it is kept as small as it can be.** Every
  line, setting, test, workflow and dependency is kept up for as long as
  it lives. Prefer the solution that leaves less to keep up.
- **A manual step is a defect.** A cache cleared by hand, a file copied
  by hand: fix the build so the step is never needed.
- **A rule a tool can check is checked by the tool,** in CI, on every
  pull request. A rule that is only written down slips.

## 7. CI stays quick

CI is where every pull request waits, and the user waits with it.

- **Every job has a short limit, five minutes as a start,** setup
  included. A job that grows past it is split or loses the work that
  does not belong in it, never given a longer limit.
- **A pull request starts only the jobs whose code it changed,** by a
  path filter on the kind of file: no backend code changed, no backend
  tests. Every pull request runs the lint and the build, because the
  repository's rules read the docs too. Inside a job the tool's own
  cache runs only what changed. A push to main runs everything.
- **A run for an old commit is stopped** when a new one comes, with
  `concurrency` and `cancel-in-progress`. Only what writes shared state
  is never stopped halfway, and CI on main keeps a result per commit.
- **Expensive machines run only what differs on them.**
- **What is long and blocks nothing runs by hand or at night,** never
  on every push.
- **Every CI job is one command,** the same one a person runs, with
  only what its machine needs around it. A check is added to that
  command, never as a step of its own in a workflow, and no script of
  ours picks, runs or reports tests.
- **A workflow that writes gets its token only from main's own copy.**
  A branch is code nobody has reviewed yet, so its copy of a workflow
  never runs with a token that writes. `pull_request_target` runs the
  main branch's workflow for a stranger's pull request with such a
  token, and one step that reads the pull request's code hands it over,
  so no workflow uses it, and a test fails on one that does.

## 8. Docs say what the code does now

- **When behaviour changes, the doc that describes it changes in the same
  pull request.**
- **Docs live in `docs/`.** Only `README.md` and `CLAUDE.md` sit at the
  top.
- **A comment says why, not what.** What the code does is in the code.
  Why it does it that way, and what went wrong before, is in the
  comment.
- **One name per thing.** Whatever a thing is called in the interface,
  it is called that in the docs, the code and the comments too.

## 9. Every change knows its speed

- **Every pull request says whether it makes things faster, slower or
  the same,** before the user has to ask: for the product, or for CI when
  only the tests or the build change. Its bottom line is one word, and
  when it is faster or slower, the change in percent with its spread,
  overall. How it is known, and each part, goes in Proof, see the
  template.
- **Ask it while working:** what runs more often or less, what waits on
  what, what is read, drawn, decoded or started again, what is held in
  memory.
- **A change on a path a person waits on is measured** before and after,
  the way section 10 writes a measurement. A session times what it can
  reach, a benchmark of the code that changed when the real thing needs
  a screen or a model it does not have. CI is measured by the times of
  the jobs the change reaches against the last runs on the main branch.
  What cannot be timed is reasoned from the code, said as reasoning,
  never as a measurement, and Proof names what was not timed.
- **Slower is not shipped** unless it buys something worth more, and the
  user accepted that trade-off. The pull request names what got slower,
  by how much and what it buys. Until the user agrees, the question is in
  their list and the pull request is not ready to merge.

## 10. A measurement a person can compare

Wherever numbers are put side by side, in a pull request, a reply or a
doc:

- Every number compared is in the same unit, with the same number of
  decimals, and the unit is said once, in the column's head.
- A comparison reads before, then after, then the change, left to right.
- The change is the difference as a share of the before, rounded to a
  whole percent, with the word that says which way it went, `20 %
  faster`, `7 % larger`. It is always a number, never `no change`.
- How much the measurement varies goes beside it, as the spread of the
  before measured more than once, `12 % faster, ± 8 %`. A comparison of
  one run each has no spread, and says so.
- Only like is compared with like. Numbers from two kinds of machine go
  in separate tables or rows.
- A sentence above the table says what was measured, where, and how many
  times.
- A table a program prints lines up in the terminal: every cell padded
  to its column's width, numbers to the right.
- Measurements are metric.

## Before saying done

1. Every fact the change rests on was checked, with its source.
2. The fix takes the cause away, in one place.
3. Nothing was copied that could have been extended.
4. Input from outside is checked at the boundary.
5. The build is clean and pinned, and CI stays under its limits.
6. The docs say what the code does now.
7. The pull request says whether it is faster, slower or the same, and
   a slower one has the user's yes.
