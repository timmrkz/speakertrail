---
name: collaboration
description: How an agent works together with the user on any project. Work in small batches that are always visible, answer a question rather than build it, keep replies short, keep what a later session needs in the repository and never in an agent's memory, give every issue a priority and a size and pick the next one by them, and write plainly. Read it at the start of a session, before replying to the user, before saving anything for a later session, and before labelling or picking an issue.
---

# Working together

The user drops a request, leaves, and comes back to skim. They have
several sessions open at once. Everything here is so that whenever they
look, they see what is going on and what they have to do, and nothing
waits on them that does not have to.

**The user** is the person the agent works for, who owns the product and
decides what it should do. **The people using the product** are the
ones it is made for.

How a pull request is opened, watched and reported on, and how every
reply ends, is the pull-requests skill. How issues are written, closed
and looked over is the maintenance skill. Who may give instructions is
section 9 of the pull-requests skill and section 4 of the security
skill.

## 1. Work that can be seen

- **Every piece of work becomes a pull request,** the way section 3 of
  the pull-requests skill says.
- **Small batches.** Split larger work into batches of a few minutes
  each. Finish, test and report after each one. Never disappear into a
  long stretch of work.
- **Just do small things.** A small, low-risk change needs no discussion
  of internal choices. Discuss constraints only for security, lasting
  effects or side effects the user would not expect.
- **What cannot be undone asks first.** A deletion, a change to what
  people using the product rely on, a message outside the project. All
  else is done, and said.
- **Ask at most one question at a time,** and only when the answer
  changes what gets built.
- **A question is answered, not built.** "Do we not have X yet?" asks
  for an answer. The agent answers it, offers the work in one line and
  waits for a yes. Work nobody asked for is not started.

## 2. Replies

- **Report plainly.** What changed, what to test and how, what is still
  open. Name the bugs found on the way, your own included.
- **Short.** The report is a few lines. Anything long is a file, linked.
- **Reply in the language of the user's message.**
- **The user tests themselves.** When the agent cannot see the product
  running, it says exactly what to look at and what should happen.
- **Every reply ends with what the work is and what the user has to
  do,** see section 10 of the pull-requests skill. For example:

  **This thread:** the search filters, made to fit on a phone (#N)
  **For you:**
  1. Open the search on your phone and add four filters. They should
     wrap inside the screen.
  2. Merge #N when it looks right.

## 3. What a later session needs is in the repository

- **Sessions forget, the repository does not.** How the user wants
  things done is a rule in the project's instructions file, a procedure
  is a skill, a fact about the system is a doc, and open work is an
  issue. Each goes in with the pull request in which it came up.
- **Never in an agent's memory outside the repository.** A note there is
  read by no reviewer, changed by no pull request, and drifts from the
  code. Where a tool offers such a memory, it holds at most a pointer to
  the repository.
- **Generic first.** A lesson about how to work goes into a skill in
  words any project can use. Only what is true of this one product goes
  into its instructions file or its docs.

## 4. Priority and size

Every open issue carries one priority and one size, beside the labels
for its kind. The priorities are the ones
[Kubernetes](https://github.com/kubernetes/community/blob/master/contributors/guide/issue-triage.md)
triages by. The sizes are the T-shirt sizes many teams on GitHub use.

| Label | What it means |
| --- | --- |
| `priority/critical` | Drop what you are doing. People's data is lost or wrong, a broken build, a failing test, a security hole. |
| `priority/important` | Worked on now or very soon. A bug a person runs into or that the code shows, and every flaky or hanging test, because a test that cannot be trusted no longer verifies what it checks. |
| `priority/backlog` | Not now. A nice to have nobody will get to soon, tidying what works, a test that is only slow, long-term work. |
| `size/small` | A short pull request in one place. |
| `size/medium` | One pull request. |
| `size/large` | More than one pull request. |

Whoever opens an issue sets both, by judgement from the table, with a
few words why. A label the repository does not have yet is made the
first time an issue needs it, with exactly the name the table gives. The
user moves any label they see differently, and their label stands.
Someone else who sees one differently says so in a comment on the issue.

### What comes next

Once the open pull request is merged, the next piece of work is the open
issue of the highest priority, the smallest first and then the oldest,
unless the user puts them in another order. Fresh evidence comes before
that order, because labels lag behind it: a test that has just failed on
main, or an issue that comes with its trace or its log, goes before an
older one with less in hand, see section 1 of the maintenance skill. The
agent says which it picked and why, see section 11 of the pull-requests
skill.

## 5. Writing

These apply to everything written for the user: interface text, docs,
commit messages, pull request text, code comments and replies.

- **Never use semicolons.** Use commas and full stops. Semicolons in
  code syntax are fine, in prose and comments they are not.
- **Plain, direct language. Short sentences.**
- **One name per thing,** the same in the interface, the docs and the
  code.
- **Numbers a person can compare,** see the pull-requests skill.
- **Thirty words first** for anything hard to explain, see the
  pull-requests skill.

## Before ending a turn

1. The work is in a pull request the user can see.
2. The reply is short and ends with what the work is and what the user
   has to do.
3. Whatever a later session needs went into the repository, not into a
   memory.
4. Everything found along the way is fixed here or logged as an issue.
