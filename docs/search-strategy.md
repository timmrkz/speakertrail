# Who to look for: a search strategy for My First Memory

A brief for the next stage of the engine. It says who makes a good guest,
where such people are found in NRW, how the engine tells them apart, and in
which order to build it. No code yet.

Speaker Trail stays the code name while the idea finds its shape. The
engine has moved from speakers at events towards people worth a
conversation.

## What we learned

Two runs found real founders, and Find on LinkedIn reached their profiles
well. So the method works: find a person, see why they count, open their
profile in one click.

What does not work yet is the fit. Many founders the engine finds build
funded tech companies. They play the funding game, wear its suits and
speak its language. For a podcast about a person's first memory that is a
poor start. It is hard for them to step out of the role and talk about
something intimate.

The guests who worked, and the ones in the pipeline, share a pattern:

| Episode | Who, in short | What fits |
| --- | --- | --- |
| 1 | owns a Brazilian jiu-jitsu gym and teaches there as a black belt | athlete, coach, owner, close to people every day |
| 2 | built a skills company inside a founder community, runs his own events, sold companies, likely never raised money | scrappy, bootstrapped, runs events |
| 3 | very young, built a house cleaning business | bootstrapped, real world, close to people |
| 4 | switched from tech to emotional coaching | coach, career switch, cares about people |

Names stay out of this file, because the repository is public.

## Who fits

**The core idea: people who work with people.** Their work is face to face,
they run it themselves, and they are used to talking about what matters to
someone.

Good fits:

- athletes, and coaches of any sport
- owners of gyms, combat sports academies, yoga and movement studios
- fitness, health, nutrition, habit and lifestyle coaches
- mental health coaches, emotional and life coaches, coaches for couples
- authors of books, above all non-fiction about life, body, mind and work
- owners of small businesses close to everyday life: cleaning, crafts,
  food, care
- founders who bootstrapped, sold companies, and run their own events,
  meetups or communities
- people who switched careers towards work with people

**Therapists are out** for now. Licensed psychotherapists have strict rules
about what they say in public. Coaches are in.

**Only NRW** for now, because the conversations are recorded in person.

## Signals the engine can read

Every signal comes with the passage that shows it, like the founder rule
today. A claim without a passage does not count.

Signals for a good fit:

| Signal | Where it shows |
| --- | --- |
| **Works with people:** coaches, teaches, trains, treats, counsels | titles, about pages, course and class pages |
| **Owner-operator:** runs it themselves, small team | imprint names an owner ("Inhaber") or one or two managing directors, the site speaks as "ich" |
| **Bootstrapped:** built from own means, no investors named | about page, interviews, no funding news |
| **Runs their own events:** workshops, seminars, open mats, meetups, retreats | event pages where they are host or organiser, not only speaker |
| **Author:** has written a book | about page, reading events, publisher pages |
| **Athlete:** competes or competed | about page, belt or title, club pages |
| **Career switch** towards people | about page |
| **Still active** | the signs of life the lookup reads today |

Signals against:

- funding rounds, investors, "Series A", "backed by"
- corporate titles like "Head of", "VP", "Chief … Officer" at a large
  company
- a large team, or a company without a person behind it
- enterprise software sold to other companies

Scoring is kept simple and readable: a list of signals, each with its
passage, for and against. The People screen sorts by it and shows why.
There is no hidden number.

## Where they are found in NRW

### Events

Today's sources are mostly startup events. The new people are on other
stages.

- workshops, seminars and open mats at gyms and academies
- readings in bookstores. The big chains and many independent shops keep
  event calendars, and the Literaturbüro NRW keeps one for all of NRW.
- health, fitness and lifestyle fairs. FIBO, the world's largest fitness
  fair, is in Cologne every April. In 2027 it runs from 8 to 11 April.
- retreats, breathwork and yoga events, running clubs
- the health, sports and self-growth categories on Meetup and Eventbrite
  in NRW cities

A shift that matters: **the host of a recurring event** is often the best
fit, not only the speaker. Someone who runs their own event for years is
scrappy and close to people by definition. The engine already knows
hosts, and they should count for more.

### Directories

Many of these people are listed, not on stage. A directory works like a
portfolio today: a list that leads to each website, then to its imprint
and about page.

- lists of gyms, like a national list of grappling gyms with a map
- the directories of the coaching associations, like ICF Germany, the
  Deutscher Coaching Verband and the DGfC, most of them searchable by
  region
- studio lists for yoga and movement

Each directory is checked against its robots.txt and its terms before it
becomes a source. A directory that forbids automated access becomes a
manual source, like any other site. No workarounds.

Directories are national. The imprint gives the postcode, and only NRW
counts.

### The person's own website

The imprint says who runs it. The about page, "Über mich" or "Über uns",
says who they are. That page carries most of the signals above: whether
they coach, whether they built it alone, whether they wrote a book.
Reading it is one more request per lookup and one question to the local
model.

## How it fits the engine

Most of the machinery exists. What changes:

1. **A fit rubric for the local model.** The model already reads event
   pages and says who is a founder, with passages. It gets a rubric with
   the signals above, and reads the about page too. The four guests serve
   as described examples, never by name.
2. **Directories as a kind of source**, next to portfolios, for lists of
   businesses run by people rather than startups. The lookup follows each
   entry to its website, imprint and about page.
3. **Owner-operators first.** The imprint rule today takes young
   companies. Sole traders and owners ("Inhaber") are the best case here,
   not an edge case.
4. **NRW by postcode**, for everything that comes from national lists.
5. **People by fit.** People opens on the best fits, with the reasons in
   one line, the way founders show today.
6. **Tim's decisions teach the engine.** Keeping or skipping someone is
   recorded already. Over time it shows which signals predict a keep, and
   the rubric is adjusted from that, by hand at first.
7. **Find on Instagram**, next to Find on LinkedIn. Coaches and athletes
   often live on Instagram rather than LinkedIn. It is a search in Tim's
   own browser, like the LinkedIn button. The engine never requests
   Instagram. The right search address still has to be checked.

## What stays the same

- The engine never requests LinkedIn or Instagram.
- robots.txt is respected, one request every 5 seconds per website.
- Only names, roles and profile links are stored, never email addresses
  or phone numbers. Coaches' directory pages often show phone numbers.
  They are not taken.
- The language model runs on Tim's Mac.
- Runs stay short.

## Order of work

Small batches, each one tested on Tim's Mac before the next.

1. **The rubric on what we have.** Score the people already found, with
   reasons, and sort People by it. This shows whether the rubric matches
   Tim's sense of fit before any new source is added.
2. **About pages in lookups.** One more page per lookup, read by the model.
3. **A starting list of new sources**, events and directories from the
   lists above, each checked by hand for robots.txt and terms.
4. **Directories as a source kind**, with the NRW postcode filter.
5. **Find on Instagram.**
6. **Learning from keeps and skips.**

## Suggestions

A **suggestion** is one person the engine puts forward as a likely guest.
It is one card: the name, what they do, the signals with their passages,
whether they are still active, and Find on LinkedIn. Tim either keeps them,
which means he wants to contact them, or skips them, and the engine does
not suggest them again.

The engine may know hundreds of people. Suggestions are the few it puts
forward each week, the best fits Tim has not decided on yet. Their number
is what Tim can review and follow up in one sitting. A proposal to start
with: 10 a week, reviewed on the phone in a few minutes. It becomes a
setting.

## How we know it works

Tim looks at the week's suggestions. Today few of them would be a guest.
The aim is that he keeps at least half. The report counts keeps and skips per signal, so a signal that
misleads shows up.

## What a session needs

The cloud session cannot reach most websites. Its network is limited to a
list of allowed domains. To build and test this against real pages, the
environment needs a broader network access level, or the domains of the
chosen sources added to its list. Tim changes this in the environment's
settings.

## Open questions

- How many suggestions a week, and on which day?
