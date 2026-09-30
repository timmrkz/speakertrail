# Who to look for: a search strategy for My First Memory

A brief for the next stage of the engine. It says who makes a good guest,
where such people are found in NRW, how the engine tells them apart, and in
which order to build it. What is built so far is under
[Where it stands](#where-it-stands).

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

The first searches found the right kind of people, owners of studios and
gyms, but most of them are not on LinkedIn. Someone who fits but cannot be
reached there does not help. LinkedIn is where people talk about their
ideas, their convictions and their business, and where Tim has reached
guests before. Instagram accounts are often quiet or all show. So the aim
is people who fit **and are on LinkedIn**, best of all people who write
there.

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

### Searches

A fixed list of sources is not enough. Most of these people are on no
list and on no stage. They have a website with an imprint, and a search
finds it, the way Tim would search "BJJ Gym Köln" himself.

A **search** is a source like any other: a query instead of a page. Its
results are websites, each followed to its imprint and about page, like a
directory's entries, and only NRW counts. The starting searches are eight
kinds of people who fit, in the eleven biggest cities of NRW:

- BJJ gyms, combat sports schools, CrossFit boxes
- yoga studios, personal trainers
- life coaches, couples coaching, nutrition advice

Tim adds more on Sources, by typing a search instead of a web address.

Search providers stand behind one interface, in `internal/search`. Why
not Google itself: its robots.txt forbids bots on `/search`, and it blocks
automated searches within a few requests. The providers are:

- **Exa**, the one to start with. Its free plan needs no card and gives
  $10 a month, about 1,400 searches at 10 results each.
- **Tavily**, whose free plan has 1,000 searches a month. Its sign-up may
  ask for a card.
- **Brave**, whose monthly credit covers about 1,000. It needs a card.

One is enough, because a run takes only three searches. Those with a key
are used together: each search goes to the one with the largest share of
its budget left, and to the next when it fails. Every call counts, also a
failed one, because it may be billed. Once a budget in Settings is spent,
the engine stops calling that provider. Another provider is one more type
with a `Search` method.

A search brings up to 20 websites, 10 from Exa. A run takes three due searches, and a
search runs again after 30 days, both set in Settings. Platforms and list
sites like Yelp, Gelbe Seiten or Eventbrite are left out of the results,
LinkedIn and Instagram always.

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

0. **The workspace for the experiment:** People live and newest first,
   events stepped back, runs made solid. Tim reviews everything that
   follows here.
1. **The rubric on what we have.** Score the people already found, with
   reasons, and sort People by it. This shows whether the rubric matches
   Tim's sense of fit before any new source is added.
2. **About pages in lookups.** One more page per lookup, read by the model.
3. **A starting list of new sources**, events and directories from the
   lists above, each checked by hand for robots.txt and terms.
4. **Directories as a source kind**, with the NRW postcode filter.
5. **Find on Instagram.**
6. **Learning from keeps and skips.**
7. **Weekly suggestions**, once the experiment works.
8. **Searches**, added once the fixed sources proved too narrow: most
   people who fit are on no list, but a search finds their website.

## Where it stands

Built, in the order above:

0. **The workspace.** People updates by itself while a run goes, newest on
   top. Runs show their true state, what they brought and what failed, in
   plain words with the action that fixes it. Events moved under More.
1. **The rubric**, in `internal/rubric`, over everyone found so far, with
   People sorted by it and every reason shown with its passage. One
   addition to the signals above: a startup's founders found through an
   accelerator's portfolio are *backed by a startup programme*, a signal
   against, because the imprint's one or two managing directors alone
   would put every portfolio founder at the top.
2. **About pages in lookups**, read by the rules and the local model.
4. **Directories** as a kind of source, with only NRW counting by postcode.
5. **Find on Instagram**, Instagram's keyword search in Tim's browser.
6. **Keeps and skips** on each person, counted per signal on Overview and
   in the report, with how the top 20 by fit were decided.
3. **The starting list of new sources**, 27 of them, each checked by hand
   for robots.txt and terms. Those that forbid automated access are
   manual sources.
8. **Searches**, with Exa, Tavily and Brave, see
   [Searches](#searches). Overview shows how much of each budget this
   month has spent.

Waiting:

- **A key.** Searches wait until `EXA_API_KEY`, `TAVILY_API_KEY` or
  `BRAVE_SEARCH_API_KEY` is set, see the README.
- **Searches that learn.** Which searches bring people Tim keeps, and new
  searches made from what those people have in common.

## First an experiment

Before anything is built for good, the strategy is tried as an experiment:
the rubric, the new sources and the tools they need. Tim reviews the
result where he does today, in the list of people.

Weekly **suggestions** come later, once the experiment shows the rubric
works. A suggestion is one person the engine puts forward as a likely
guest: the name, what they do, the signals with their passages, whether
they are still active, and Find on LinkedIn. Tim keeps them, to contact
them, or skips them, and a skipped person is not suggested again. How
many a week is decided then.

## The workspace for the experiment

Tim reviews on People while runs go. What that needs:

- **People updates by itself while a run goes.** New people appear
  without a refresh and without coming back later.
- **Newest on top** is the default order of People. A new person is what
  Tim looks at first.
- **Events step back.** They stay, as a source of people and for the
  public calendar, but they are no longer prominent in the workspace.
- **Runs need care.** Starting, watching and stopping a run feels
  rudimentary and brittle today. It has to feel reliable and solid:
  - a run always shows its true state: going, finished, stopped, or ended
    by a restart, and never a step that is no longer true
  - what it does now, by name, and what is left, with the time left
    measured, never stuck on a vague waiting message
  - when it ends, one line on what it brought: new people, new fits,
    startups looked up, and what failed
  - failures grouped by source, with the reason in plain words and the
    one action that fixes it, like retiring the source
  - Start and Stop answer at once and never leave the screen in between
  - the same state on every screen, the phone included, without a refresh

## How we know it works

Tim looks at the newest people after a run, and at those sorted by fit.
Today few of them would be a guest. The aim is that at least half of the
top 20 by fit are people he would contact. The report counts keeps and skips per signal, so a signal that
misleads shows up.

## What a session needs

The cloud session cannot reach most websites. Its network is limited to a
list of allowed domains. To build and test this against real pages, the
environment needs a broader network access level, or the domains of the
chosen sources added to its list. Tim changes this in the environment's
settings.

## Open questions

- How many suggestions a week, and on which day? Decided after the
  experiment.
