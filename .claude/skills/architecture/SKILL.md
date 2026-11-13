---
name: architecture
description: How to keep and change the architecture of any project, where each kind of thing belongs, and the server and client split we build toward, where the backend owns every rule and all state and the client only renders. Read it before any change that adds a new kind of thing, a part, a home for state, a boundary or a call between client and server, before deciding where a piece of code goes, and before reviewing the design of a change. Where another skill says otherwise about design, this one wins.
---

# Architecture

The architecture is where each kind of thing belongs, and what must stay
true. It decides where a piece of code goes before anyone writes it. Code
can work and still put a rule in the wrong place, and that is how a
design wears away, one pull request at a time, each of them green.

Two people are named here, as in the testing and review skills. **The
user** is the person the agent works for, who owns the product and
decides what it should do. **The people using the product** are the ones
it is made for. A project writes its own architecture down, in
`docs/ARCHITECTURE.md`. This skill says how such a file is written, kept
and changed, and what shape we build toward.

## 1. The architecture is written down

- **One file, short, about what rarely changes.** Its shape is
  [matklad's ARCHITECTURE.md](https://matklad.github.io/2021/02/06/ARCHITECTURE.md.html).
  It says where things are and what must stay true, never how a part
  works inside. That is the part's own doc.
- **It holds these sections, in this order:**
  - **Bird's eye view.** The parts as layers, each using only the ones
    under it, with a drawing.
  - **Code map.** One row per part: where its code is, what it owns, and
    what it never holds.
  - **Where state lives.** Every piece of state, its one home and its
    one writer.
  - **Where does it go?** A table from "I need to" to the place it goes,
    for the changes people make most.
  - **Invariants.** What must stay true, and what holds each one: a check
    in CI, or the review.
  - **Boundaries.** Where one layer meets another, so a change there
    says what the other side relies on.
  - **Not to copy.** Code that breaks the map today, each with the issue
    that fixes it.
- **The layers are drawn, not only listed.** Each drawing is plain SVG
  in `docs/diagrams/`: the layers as boxes from the top down, an arrow
  down the side for "may use", the path under each box's name, and its
  parts as small chips. It follows the reader's light or dark theme
  with `prefers-color-scheme`, on a background of its own, and names
  only paths that exist. A change that moves a box changes its drawing
  in the same pull request.
- **A line is added only for a new kind of thing:** a new part, a new
  home for state, a new boundary. A change that brings one adds its line
  in the same pull request. Rewording is not a change of the
  architecture.
- **Every path the file names exists.** A check in CI holds it once
  there is one, and the review until then.
- **A pull request that closes an issue the file names** rewrites the
  rows that name it and removes its line under Not to copy.

## 2. Layers, and which may use which

- **A part may use the parts under it and knows nothing of the parts
  over it.** That direction is the architecture. A part that knows one
  over it can no longer be changed, tested or replaced alone.
- **A rule belongs as low as it can go,** where every caller reaches it.
  A rule found higher up, in a front end or the interface, is in the
  wrong place, however convenient it was there.
- **A rule is placed by what it is about,** not by where it was needed
  first.
- **The language holds what it can.** Imports that may not cycle,
  packages that may not reach each other, a module boundary. What the
  language cannot hold, the review holds, and the file says which.

## 3. State has one home

- **Every piece of state has one home and one writer.** A second copy is
  a bug waiting for the day the two disagree, and no test finds every
  order of events that makes them disagree.
- **A cache is a copy.** It names the home it copies, and how it learns
  that the home changed. A cache that cannot say that is a second home.
- **What is derived is derived, never stored beside its source.** A count,
  a filtered list, a sorted view are worked out from the state when they
  are needed, by the part that owns the state.
- **State that outlives one request or one job** has a home that lives as
  long, made once and handed in, never a global or a package variable.

## 4. The shape we build toward: the server owns, the client renders

Our products are built as a server and a client. The server, the
backend, holds every rule and all the state. It is the source of truth.
The client, the frontend, shows what the server sends and passes on what
the person did. It is a renderer, and nothing more.

| Part | Owns | Never holds |
| --- | --- | --- |
| storage, a database once there is one | the state, durably | a rule |
| the server's domain core | every rule: what is valid, what follows from an action, what is shown, filtered, sorted, counted | anything about a screen, a request or a click |
| the server's API | turning a request into a call on the core, and the core's answer into a response | a rule of its own. It asks the core and passes the answer on |
| the client | drawing what the server sent, and sending what the person did | a rule, a decision, or state of its own beyond what the hand is doing right now |

What follows from it:

- **The client asks, the server answers.** The client sends an intent,
  "add this tag to this item", never a result it worked out, "here is
  the item with its new list of tags". The server validates it, applies
  it, and sends back the new state or what changed.
- **The client keeps no state of its own.** What it shows is what the
  server last sent. The only state it holds is what the hand is doing
  right now: a field being typed, a drag in progress, which panel is
  open. Once the action ends, it goes to the server, and the server's
  answer replaces it.
- **No rule is copied into the client.** Not to save a round trip, not
  for an instant answer, not with a shared case file to keep the two in
  step. A copy starts out the same and drifts. Where the client must
  answer while a hand moves, the server sends what that answer needs as
  data, and the client only looks it up.
- **Validation lives on the server.** The client may mark a field the
  server said is wrong. It never decides by itself that a value is
  valid, because a client is not to be trusted, and a check done twice
  has two answers.
- **The server is tested where the rules are.** A rule is tested in the
  core, as a unit, without the API or a browser. The client's tests check
  what a person sees and does against a real server, see the testing
  skill.
- **The API is a boundary.** Its shapes are written once, on the server,
  and the client's types are made from them, never written by hand
  beside them. A change to it says what the client relies on.
- **The database comes when it is needed, and only the server sees it.**
  Plain files are fine until two writers, a query or a size make them
  wrong. The client never knows what the storage is.
- **A client that starts with logic in it** is the state to move away
  from, not a pattern to follow. Its rules are listed under Not to copy,
  each with the issue that moves it to the server.

## 5. Placing a change

Before writing code, and again in the review:

1. **Read the architecture file,** not only the code beside the change,
   which may be the very code Not to copy names.
2. **Name every new thing the change adds:** a function, a type, a rule,
   a piece of state, a call across a boundary.
3. **Place each one** in the layer and part the file gives it. If the
   file has no place for it, that is a new kind of thing, section 6.
4. **Weigh the change as a whole:**
   - **Better:** it moves something to where it belongs, removes a copy,
     or closes a breach Not to copy names.
   - **The same:** everything it adds is where the file says.
   - **Worse:** it adds to a breach, builds on one, puts a rule in a
     layer that should not hold it, copies a rule across a boundary,
     gives state a second home, or lets a part know one over it.
5. **Worse is fixed in the same pull request,** never left for an issue.
   A breach that was there before is not this change's, unless the
   change builds on it or adds to it.

## 6. Changing the architecture

A change to how the code is organised or designed is worked through
several times before any code. It is a change that decides something the
architecture file records: a part, a layer, a home for state or a
boundary added, removed, split or joined. Rewording a row is not one.
Carrying out a design a decision record already holds is not one either,
it names that record. When in doubt, it is one.

1. **Five simulations.** Each is a candidate design from a principle of
   its own, worked through on the code as it is, as if it were built: the
   layers it gives, what moves where, the names that would cross each
   new boundary, counted from the code and not guessed, the next likely
   changes and what they would touch, and the cost in files and pull
   requests. Five variations of one idea are one simulation. Five
   principles make a good start: who may know about whom, what changes
   together, where state lives, what runs together, and what a test has
   to build.
2. **They converge.** What comes back in most of them is kept, what only
   one needs is dropped or kept for a reason that is said. The final
   candidate is drawn as layers, each using only the ones under it.
3. **It is written down** as a decision record in `docs/decisions/`,
   pushed with the first code of the change. A change made over several
   pull requests has one record, in the first, and the others name it.
4. **The review agent gets the record** and reasons through the five
   itself before it reads the verdict, see the review skill.

### Decision records

The shape is Michael Nygard's
[architecture decision record](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions),
with the five simulations in the middle. A file is named by the date and
what it decides, `2026-11-03-search-filtered-on-the-server.md`, never by
a number, because two pull requests open at once would take the same
number. It holds:

- **The problem**, in the words of the person who asked, and what was
  measured on the code as it is.
- **Five simulations**, one section each.
- **What converges**, what came back in most of them and what in only
  one.
- **The decision**, the final candidate drawn as layers.
- **What follows**, what it costs, what it buys, and what would show it
  was wrong.

A record is not rewritten after its pull request is merged. A later
decision that undoes it is a record of its own, which names the one it
replaces, and the old record gains one line at its top, Replaced by,
with a link to the new one.

## 7. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| validate in the client too, for a quick answer | two checks give two answers the day one changes | the server validates, the client shows what it said, section 4 |
| keep a copy of the state in the client and sync it | two homes for one state, and every order of events is a new bug | the client shows what the server last sent, section 4 |
| an optimistic update that the client works out | the client then holds a rule | the client shows that the action is under way, the server's answer shows the result |
| follow the pattern in the code next to the change | that code may be what Not to copy names | the architecture file, section 5 |
| write a decision down after it is built | the alternatives are gone by then | five simulations before the code, section 6 |
| a database from the start | a moving part nothing needs yet | plain files until they are wrong, section 4 |

## Before saying done

1. Every new thing the change adds is where the architecture file says.
2. A new kind of thing has its line in the file, in the same pull
   request.
3. No rule and no state was added to the client.
4. Nothing was copied from what Not to copy names.
5. A change to the design has its decision record with five candidates.
