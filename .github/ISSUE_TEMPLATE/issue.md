---
name: Issue
about: Something that went wrong, the cause behind several of them, or work that was asked for
---

<!--
Sessions open issues through the API, which shows no template, so a
session reads this file and writes the issue the way it lays it out.
Plain words, short sentences, no semicolons. See the collaboration
skill in .claude/skills/ for how issues are labelled, ordered and
looked over.

WHEN TO OPEN AN ISSUE, AND WHEN NOT

- Not for what the pull request you are on caused, or a small thing on
  the code it touches. Fix it there, as a commit of its own, see Clean
  up now, not later, in the collaboration skill. An issue starts a whole
  new round of work.
- Only for what someone saw happen: the user, a person using the
  product, or a test. Say which. When only a test's environment causes
  it, a CI runner's virtual hardware, a stand-in or a file made for the
  test, the issue is about the test, see section 6 of the testing skill.
  An order of events in our own code that a test can make happen, a
  page closed in the middle of a save for example, counts even if nobody
  has seen it happen yet.
- Search the open and the closed issues first. The same failure as an
  open issue goes into that issue.

THE THREE KINDS

- An incident: one time something went wrong, a bug the user saw, a CI
  run that failed, a test that hung. Most issues are one.
- A problem: the cause behind two or more incidents. It is labelled
  `problem`, and its incidents are its sub-issues on GitHub. It closes
  last, after its last sub-issue, and is reopened when the cause comes
  back.
- Work: a feature or a change the user asked for. Work that takes more
  than one pull request has a Progress list at the top, one line per
  pull request, ticked with its number as each merges.

The title says what goes wrong or what is asked, in plain words, as a
sentence a person would say, never a name from the code alone. Every
issue carries one kind label or more, `bug`, `feature`, `tests` for
tests and CI, `reliability`, `performance` or `security`, and one
priority and one size, as the collaboration skill defines them. A
problem carries `problem` too. Leave out a section that has nothing to
say.
-->

<!--
No heading here. At most 30 words: what goes wrong or what is asked,
who saw it happen or asked for it, and where. For example: "On a phone,
an item edited offline is gone after the page reloads. The user saw it
testing #10 on 3 November."
-->

## Progress

<!-- Work over several pull requests only. One line per pull request. -->

- [ ]

## Seen

<!--
Where it was seen, so anyone can find it again: the user's report with
its time and build, the log lines that show it, a CI run with its link
and commit, or the review or the code it was read from. A problem lists
its incidents here instead, and every fix tried so far with why the
cause stayed. Work quotes what the user asked, in their words, with the
date.
-->

## Cause

<!--
At most 30 words: why it happens, traced to the file and the name in
the code, or "Not known yet" and what would find it, the trace first.
-->

## What to build

<!--
At most 30 words: the change that takes the cause away, in one place,
and when it is done. Never a workaround, see Root causes only in the
engineering skill.
-->
