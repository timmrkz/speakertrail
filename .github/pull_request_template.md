<!--
How a pull request is described, the one place that says it. Plain words,
short sentences, no semicolons. The opening, Root cause and The fix are
brief and concrete: at most 30 words for each issue the pull request
works on, one short paragraph per issue. A pull request for three issues
has three paragraphs in each of the three. Proof may say as much as it
needs. The bottom line, How to test and Found along the way are always
there. Leave out any other section that has nothing to say. See the pull-requests skill
in .claude/skills/.
-->

<!--
No heading here. For each issue, at most 30 words: what goes wrong or
what was asked, who saw it happen and where, and what gets better. Then
`Fixes #N` for each issue this finishes, so merging closes it. For
example: "Fixes #12. On a phone, the search filters ran off the screen
and the last one could not be removed. The user found it testing #10.
Now they wrap inside the screen."
-->

- **Architecture:** <!-- better, the same or worse -->
- **Speed:** <!-- faster, slower or the same -->

<!--
The bottom line, always, and nothing else in it. Architecture: one word,
better, the same or worse, against the architecture as the project
writes it. Speed: for the product, or for CI when only the tests or the
build change. One word, and when it is faster or slower, the change in
percent with its spread, `12 % faster, ± 3 %`, or `one run` in place of
the spread. It is the change overall, on what a person waits on. A part
that got slower is said in Proof, and its trade-off waits for the user's
yes in the reply. The same only when the change adds and takes away no
work that runs. A measured change is its number, even inside its
spread, `2 % faster, ± 5 %`. Everything else goes in Proof: what was
measured, where and how often, the parts, CI when the product changed
too, what was read from the code, where each new thing went, and what a
slower part buys. See the engineering skill on speed.
-->

## Root cause

<!--
For each issue, at most 30 words: why it happened, traced to the file
and the name in the code. Where it was seen says whose fault it is: a
failure seen only where a test runs is the test's or its environment's,
see section 6 of the testing skill. A pull request that adds something
says what it builds on instead.
-->

## The fix

<!--
For each issue, at most 30 words: what changes, with the files and the
names in the code. The docs that changed with it.
-->

## Proof

<!--
The tests that ensure it, and that they fail on main and pass here. The
measurements before and after that the speed line stands on, and
whether CI passed on the last commit.
What could not be checked here is said. This section may be as long as
the proof needs.
-->

## How to test

<!--
A numbered list of steps the user takes to see the change work, each one
they can take as it stands, where the change shows: the page, CI, the
repository. What to do, what to look at and what to verify. When there
is nothing to test, one line instead of the list: "Nothing to test,
because" and why, for example that only a test changed and CI runs it.
-->

1.

## Found along the way, not fixed here

<!-- Each with the issue it was logged as, or "Nothing". -->
