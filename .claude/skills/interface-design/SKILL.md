---
name: interface-design
description: How to build and change the interface of any product, above all when you cannot see it yourself. What earns a place on screen, how a click, a drag and slow work are answered, consistency and design tokens, layout left to the stylesheet, and how to measure and prove a visual fix instead of claiming one. Read it before any change to what a person sees or touches, and before answering a report that something looks or feels wrong.
---

# The interface

The interface is the mechanism, not the machinery. It exists so a person
can do what they came to do, and every part of it earns its place by
serving that, in the simplest form that does. It draws what the server
sent and passes on what the person did, and it holds no rule of its own,
see the architecture skill.

Two people are named here, as in the other skills. **The user** is the
person the agent works for, who owns the product and decides what it
should do. **The people using the product** are the ones it is made
for. A session usually cannot see the screen the user sees, so this
skill is as much about proving what is on screen as about what should
be.

## 1. What goes on screen

- **Only what serves the person.** What the code needs in order to work
  is not a feature: a number it computes, a state it keeps, a step it
  takes, none of that belongs on screen just because it exists. A control
  that serves no one does not go in, and comes out again if it is in.
- **Whatever one click does, one click undoes.** Everything saves at
  once, with no save buttons, and is undone as easily as it was done.
- **What cannot be taken back asks first,** in one kind of dialog, the
  same everywhere. It opens on the safe answer, and the buttons read from
  the safest to the one that cannot be taken back. The destructive one is
  never where the keyboard starts and wears the colour of a warning. What
  can be undone never asks, it shows what it did.
- **Help waits until it is asked for.** No line of instructions sits on
  screen for good. What a control is for goes in its tooltip, anything
  longer behind a small info mark that opens on hover or click, on the
  area it explains. Text that is always there is text nobody reads, and
  it moves the controls as it comes and goes.
- **What the product can do by itself, it does.** A step every person
  takes, the product takes for them. A state every item passes through,
  like not being ready yet, is never reported as a failure.
- **Borrow what people already know.** Where the platform, or the apps
  people use every day, already have a way of doing something, an icon,
  a word, a place for a control, a flow, do it their way. Look it up
  before choosing. A choice of our own is made only where there is no
  convention, and the reason is said where it is made.

## 2. A person's action is answered at once

- **Everything a click changes shows the change in the same frame,** the
  control and every place that shows the same thing. Say what is under
  way, Saving, Stopping, Starting, and the server's answer replaces it
  when it comes. Never wait for an event or a timer to show what the click
  already knows. The control takes no second click while it works. This
  shows that an action is under way, never a result the client worked out
  itself.
- **What is being dragged is followed while it moves,** not when the hand
  lets go. List everything drawn from the dragged thing, and make each
  follow the draft. When what follows is the server's to work out, ask on
  the way, one question at a time, always about where the hand is now, and
  keep the last answer until the saved state comes back, so nothing jumps
  as the hand lets go.
- **Everything drawn from the dragged thing comes in one answer.** The
  list is easy to make one item short. A part made to follow the hand,
  drawn between two others that still read the saved state, runs from
  where the hand is to where nothing is any more. Let the server answer
  the hand with everything drawn from the thing, in the shape the saved
  state is sent, and draw every part from that one answer. A part read
  from anywhere else is the one left behind.
- **Work a click starts is built in front of the person.** What it asks
  for is shown where it will live from the moment it is asked for, brought
  into view, and filled in as each part becomes known. Nothing appears
  whole after seconds of nothing.
- **What is taken away is seen going.** A removed row keeps its place for
  a moment, says what became of it and offers itself back, and only then
  does the list close over it. Nothing vanishes under the pointer.
- **A thing that changes what it is stays one thing.** When an item turns
  into another, a draft into a saved item, keep its identity, its key in
  a list, so the eye sees one thing change and not one leave and one come.
- **A list that work fills is filled where it stands.** New rows come in
  without shuffling the rows already there.

## 3. Where outcomes are said

- **An outcome stays where its work was.** How a piece of work ended is
  said where it was started and where its result would have appeared,
  and it stays until it is acted on, after a reload too.
- **No banners.** A banner is standing text that moves everything under
  it. What goes wrong with no place of its own on screen is a toast, one
  component, over the content and moving nothing. Its time runs down from
  the moment it comes, stands still under the pointer, and its cross takes
  it away. Every toast is also in the log.
- **Why something failed is said whole,** never cut to a line in a row.
  A field typed wrong is still said at the field.
- **Work called off by hand is said the same way as work cut off,**
  stopped, with a way to continue, because what it did stays.

## 4. One look for one kind of thing

- **Consistency over novelty.** A visual treatment used in one place is
  used for every equivalent element, or not at all. Two things of the same
  kind are the same size, in the same colour, in the same place. A
  difference between them reads as a hierarchy that is not there. Before
  adding a size, a colour or a spacing, find the thing it is a sibling of
  and take that one's.
- **Design tokens, not numbers.** Colours, sizes, gaps and control
  heights come from one set of tokens in the stylesheet. Any other number
  needs a reason that can be said out loud. One gap between two things,
  everywhere. Controls in one row have the same height.
- **One accent, mixed, never set by hand.** Its lighter shades and its
  wash are mixed from it.
- **A button fits its words,** padded the same on either side, never
  given a width of its own. When its words change, its width changes.
- **One component for each kind of control,** used everywhere, never
  copied: one list to pick from, one dialog, one toast, one way of
  showing work in hand and how far it has come. A copy made to look alike
  drifts the day either is changed. A control the system draws in its own
  look, like a native select in a dark interface, is replaced by our own
  built on an accessible headless library, which brings the keyboard, the
  roles and the focus.
- **A menu's rows run from edge to edge.** The highlight of the first and
  the last row meets the panel's edge, with no strip of nothing.

## 5. Layout is the stylesheet's work

- **Room that is there is used.** A bigger window makes the interface
  bigger, never emptier. Before anything is laid out it is decided what
  may grow: a picture grows until it runs out of height or width and keeps
  its shape, a list, a text and a chart grow with the room, a button, an
  icon and a small field stay their size. What may grow shares what the
  rest leaves, and the sum is always the whole window.
- **No JavaScript in the resize path.** Code that measures a size, works
  another out from it and writes it back lands a frame or more late, and
  two parts that measure each other never settle. Every size is one
  expression in the stylesheet, from the viewport and the tokens. What the
  stylesheet cannot know is one custom property, set where it changes.
- **Everything lands on a whole pixel.** Something between two pixels is
  painted one way normally and another the moment opacity, a transform or
  a filter gives it a layer of its own, so it steps as it fades. A canvas
  draws in device pixels and rounds.
- **It works at every size people use,** a phone in the hand included,
  with no horizontal scroll and every control reachable.
- **Accessible by default.** Every control is reached and used with the
  keyboard, has a name a screen reader reads, and shows where focus is.
  Text and controls meet the contrast of WCAG 2.2 AA. Motion respects
  `prefers-reduced-motion`.
- **A floating list keeps what it hangs from in place.** A container that
  opens on hover stays open while a list or a bubble of its own is open,
  and after it closes asks where the pointer is instead of trusting what
  it last knew.

## 6. Measure the right thing

A session that cannot see the screen still has a browser it can drive.
It opens the interface there, against a fake server where needed, and
measures. Decide what kind of bug it is before picking the instrument:

| The thing is | Measure with |
| --- | --- |
| in the wrong place, the wrong size | the layout boxes, `getBoundingClientRect` |
| the wrong colour, weight, brightness, or not lined up on screen | the pixels of a screenshot |
| moving when it should not | both, and compare |
| happening at the wrong time | the state on every frame |

- **A layout box cannot see a painting bug.** An element painted in the
  wrong place still reports the right box, and a box is not the ink in
  it: two names can end at the same box edge and a pixel apart on screen.
  Whether things line up is a question about painted pixels.
- **While anything glides, the box is the animation, not the value.**
  Read the style the code set beside the box, or measure once the
  transition says it has ended, never after a pause.
- **A moving target is not clicked at a point worked out a moment ago.**
  Press the element itself when the handler is what is tested.
- **What happens between frames is read on every frame.** Act inside the
  page and read the thing on every `requestAnimationFrame` for a second,
  then print the runs of frames that looked the same. A run of the state
  before the click is the bug. A screenshot never shows it.
- **A drag is read at each step,** every part drawn from the dragged
  thing, before the hand lets go. A number that only changes after the
  release is the bug.
- **The fake is as slow as the real thing.** A fake server that answers at
  once hides exactly the gap a person sees.
- **The engine people use.** A browser engine of its own, WebKit on
  Apple's platforms, differs in typing, focus, hover, painting and timing.
  Run the steps there too before saying it works there, and run a probe
  there too, not only the tests. A probe that passes in one engine says
  nothing about a bug a person sees in the other.

## 7. Prove it, do not claim it

1. **Reproduce it first.** If you cannot make the bug happen, you cannot
   know you fixed it. When it does not reproduce, say so rather than
   implying a fix.
2. **Put the old code back and measure again.** A number that changes
   when the fix is removed is a fix. A number that does not is a
   coincidence, or a probe that could never reach the broken state. A
   probe that has never failed is not yet evidence.
3. **Check against what the person said.** A precise report is evidence,
   and its pattern, where it happens and where it does not, often finds
   the cause faster than reading the code.
4. **The fix comes with an end-to-end test** that does what the person did
   and checks what they saw, written the way the testing skill says. It
   fails on main and passes with the fix.
5. **Visual choices iterate fast.** Render the candidates side by side
   from the real code, light and dark, at a phone's width and a laptop's,
   rather than one build per idea, and compare with the version before.

## Before saying done

1. Everything on screen serves the person, and one click undoes one
   click.
2. Every action shows in the frame it lands in, and a drag is followed
   while it moves.
3. Every new look is an existing one, from the tokens and the shared
   components.
4. Layout is the stylesheet's, on whole pixels, at every size, with the
   keyboard and a screen reader.
5. The bug was reproduced, measured with the right instrument, gone with
   the fix and back without it.
6. The reply says what to look at and what should happen, and names what
   could not be verified here.
