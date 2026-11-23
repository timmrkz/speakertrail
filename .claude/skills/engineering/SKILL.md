---
name: engineering
description: What good engineering looks like in any project, the first principles every change follows. Think and verify before building, standard paths and known answers, one primitive per thing, failures contained, one command to build, few pinned dependencies, no logic in shell, a port that copies its source, and docs that say what the code does now. Read it before any change to code or build, and before fixing anything. The skills it points to hold the rest.
---

# Engineering

Good engineering is the simplest change that makes the product better
for the people using it, made where it belongs, proven, and cheap to
keep. Everything below follows from that.

The other skills each hold one part, and this one points to them:

| For | Read |
| --- | --- |
| where a change goes, the server and client split | the architecture skill |
| evidence before work, root causes, problems that come back, issues and triage | the maintenance skill |
| how a change is proven, and how a bug is found | the testing skill |
| how CI stays fast, trustworthy and cheap | the ci skill |
| untrusted input, paths, programs, secrets, the supply chain | the security skill |
| sizing, describing and watching a pull request, its performance line, numbers a person can compare | the pull-requests skill |
| how a change is judged | the review skill |

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
- **Standard, supported paths only.** A clever trick few others use,
  like writing to another program's private socket, is not built, even
  when it is faster. It breaks with the next update and nobody else
  keeps it working.
- **Known problems have known answers.** A cache, a retry, an undo, an
  update flow, an animation curve: look up how established software does
  it before inventing. Where people already know a way of doing
  something, an icon, a word, a place for a control, do it their way.
- **Ask at most one question at a time**, and only when the answer
  changes what gets built. Everything else is decided, and the decision
  said.

## 2. Root causes only

Every fix takes the cause away, never only what it did this time. How
the evidence is weighed, who a problem happens to, when to stop fixing
and capture instead, and what a second fix of one symptom means, is the
maintenance skill, sections 1 to 4. How the cause is found is the
testing skill, section 6.

- **A fix makes the design simpler, or it is the wrong fix.** Ask
  whether it streamlines the design or adds an exception that works
  around what is there. A fix that is generic and sound makes the next
  case of the same kind right by design.

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

## 4. When things go wrong

- **A failure keeps its kind.** An error is returned to its caller with
  what went wrong and where, never swallowed, never turned into a
  default that hides a broken invariant.
- **A failure fails its own work, not the program.** A request or a job
  that goes wrong ends with its reason, and everything else carries on.
- **Logs make the product testable in the field.** Structured logs, from
  an established library, at the places a bug would be found, tagged
  with what they are about. One report from a person using the product
  should be enough to find the bug, without a second round.

## 5. Build and keep

- **One command, from a fresh checkout to a running product.** Nobody
  should have to remember a second one. The same command runs on a
  laptop, in a session and in CI, see the ci skill.
- **A build finishes without warnings,** and CI fails on one. A warning
  lived with is a warning nobody reads.
- **Versions are pinned,** the language, the tools, every dependency and
  every action in a workflow, to an exact version or commit. Raising one
  is a line in a pull request, where it can be read first. How they are
  watched is the security skill, section 3.
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
- **No shell scripts.** What a build or a workflow has to work out is a
  program in the project's own language, with tests. A workflow step
  only calls tools that are there. Nobody reads or tests logic in shell,
  and it piles up.
- **A port copies its source as it is.** Bringing a design over from
  another project means copying it, changing only what the new project
  makes necessary, and naming every difference before the user finds
  it. Copy from the source's main as it stands that day. Diff the port
  against its source before calling it done. A flaw a review finds in
  ported code is checked in the source too, and the user is told whether
  it was copied or came with the port. A copied flaw gets a short
  briefing for the source project's sessions.

## 6. Docs say what the code does now

- **When behaviour changes, the doc that describes it changes in the same
  pull request.** Search the docs, comments, decision records and
  templates for every renamed term and changed behaviour, see the
  maintenance skill, section 7.
- **Docs live in `docs/`.** Only `README.md` and `CLAUDE.md` sit at the
  top.
- **A comment says why, not what.** What the code does is in the code.
  Why it does it that way, and what went wrong before, is in the
  comment.
- **One name per thing.** Whatever a thing is called in the interface,
  it is called that in the docs, the code and the comments too.

## Before saying done

1. Every fact the change rests on was checked, with its source.
2. The fix takes the cause away, in one place.
3. Nothing was copied that could have been extended.
4. The build is clean and pinned, with one command and no logic in
   shell.
5. The docs say what the code does now.
6. The skills this one points to were read for their part of the
   change.
