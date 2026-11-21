# The fit score: who comes first, and how the app learns from Tim

A spec for issue #68, written on 21 November 2026 before any code. It
says how People is ordered, what a person's **fit** is made of, how it
learns from whom Tim connects with, and how that steers the searches.
Tim agreed on it on 22 November 2026. The four fixed parts are built,
with People sorted by the fit. Like your picks and the searches are not
yet, see the Progress list of the issue.

## What Tim asked for

Tim, 21 November 2026: one measure of how well a person fits, with many
criteria, that orders the lists so he picks the best from the top. Age
already counts. A person who posted 5 days ago comes before one who
posted 5 months ago. A person near him comes before one far away. And
whom he connects with, and whom not, teaches the measure and the
searches, so the picks get better over time.

## What there was before the fit

- **The rubric**, `internal/rubric`, finds signals for and against a
  fit, each with the passage that shows it. The **score** is the
  signals for less those against, so a person has 1, 2 or 3 points.
- **Likely under 30** is one of those signals, guessed from the years
  on the profile. Older is not counted against.
- **Active on LinkedIn** is yes or no: anything in the last six months.
  Whoever is quieter is not kept at all.
- **The place** is only a gate: in NRW or not. Köln and Bielefeld count
  the same.
- **People** sorts by youngest first by default. Best fit sorts by the
  score, which ties almost everyone.
- **Searches** learn a little: a run takes the searches that led to the
  most connections first, `RankSearches` in `internal/store/rank.go`.
  Whom Tim connects with teaches nothing else.

## The fit

The **fit** is a number from 0 to 100. People, the newest people of a
run and every list of people are ordered by it, best first. Every point
has a reason Tim can read on the person, under Why. There is no hidden
number.

It has five parts. Four are fixed and count the same, a quarter each,
because nothing yet says one matters more. The fifth, **Like your
picks**, learns from Tim and grows as he connects with people.

### 1. Who they are

The rubric's signals, as today, without age and activity, which have
parts of their own. Works with people, Built it from own means, Runs
their own events, Author, Athlete and Switched careers towards people
are for. Raised money, Sells to companies and Backed by a startup
programme are against.

Each signal for is a third of the part, each against takes a third
away, between none and all of it. Runs their own business is not
counted, since everyone kept has it.

### 2. Younger

| Likely age | Part |
| --- | --- |
| 30 or younger | all of it |
| 31 to 39 | a tenth less for every year past 30, so 35 has half |
| 40 or older | none |
| not known | half |

An age the headline gives beats the guess, as today. Nobody is left
out for age.

### 3. Active lately

From the newest post, comment or reaction the profile shows. The part
halves every 30 days:

| Last active | Part |
| --- | --- |
| 5 days ago | 89 % |
| 30 days ago | 50 % |
| 60 days ago | 25 % |
| 5 months ago | 3 % |

Writing says more than a like. A reaction counts half as much as a post
or a comment of the same day, so a comment a week ago can count for
more than a like yesterday. The kind is read from the profile's
activity lines, `rubric.Writing`: an entry like "Gefällt mir" or
"Likes" is a reaction, anything else, like "Kommentar", is writing. So
a word for a reaction the app does not know counts as writing, as all
activity did before. Whom the app has no profile text of counts as
writing too. The words Exa's index uses were seen only in the tests, a
cloud session cannot reach Exa, so Tim's own people are the check.

The date is what Exa's index last saw, which can be weeks old. It is
refreshed whenever a search finds the person again.

### 4. Near you

Settings gets **Where you are**, a city in NRW. The distance is a
straight line between the two cities, from a table of the cities the
app knows in `internal/place`, with no request to any map. The part
halves every 40 km. From Köln, for example:

| Person in | About | Part |
| --- | --- | --- |
| Köln | 0 km | all of it |
| Bonn | 25 km | 65 % |
| Düsseldorf | 35 km | 55 % |
| Dortmund | 75 km | 27 % |
| Münster | 125 km | 11 % |
| Bielefeld | 160 km | 6 % |

A person whose profile names only a region, like "Ruhr Region", or no
city at all, gets half. Until Tim sets Where you are, everyone gets
half and the part changes no order.

### 5. Like your picks

What the people Tim connected with have in common, against the people
he rejected. Each person is described by a few plain **traits**, all
read from what the file already holds:

- each rubric signal they show
- each search that found them
- their city, and how far away: under 20 km, 20 to 50 km, further
- their age: under 25, 25 to 29, 30 to 39, 40 or older, not known
- how lately active: a week, a month, three months, longer, and
  whether they write or only react
- the words of their headline and roles, like "Yogalehrerin" or
  "BJJ", in lower case, without short words
- how many follow them: under 500, 500 to 5000, more
- whether the profile has an About

Only people Tim decided on teach it, those he connected with and those
he rejected. Open people are his to-do, not a no, so they teach nothing.
For each trait the app counts how often Tim connected, rather than
rejected, people who have it, against how often he connected overall. A
trait twice as common among his connections counts for, one half as
common counts against. A trait fewer than 3 decided people have counts
nothing. Every count starts with 5 people's worth of doubt, so a trait
seen once or twice moves little. The 5 strongest traits of a person add
up to the part, from none to all of it. Until Tim has both connected
with and rejected someone, every person gets half.

**It grows with Tim's choices.** The part's share of the 100 points
starts at nothing and grows with the number of people Tim connected
with or rejected, towards at most 40:

| People decided | Like your picks | Each fixed part |
| --- | --- | --- |
| 0 | 0 | 25 |
| 20 | 20 | 20 |
| 60 | 30 | 17.5 |
| 180 | 36 | 16 |

So the order is plain at first and leans on Tim's picks as they show
something. The cap keeps the four fixed parts above half, so a person
unlike anyone before still gets seen.

### How it reads on screen

A row in People shows the fit as a small figure in tabular figures. The
person shows the fit under Fit, above Why, with the points each part
gave and why, for example:

- +11 Who they are: Works with people, Athlete
- +20 Younger: Bachelor from 2019, so about 26
- +18 Active lately: Wrote on 16 November 2026
- +11 Near you: Bonn, about 24 km
- +9 Like your picks: "BJJ", 4 of 6 such people you connected with

Why keeps the rubric's signals with the passages that show them, so an
age or a date can show in both, once as points and once as the passage.

Signals against keep their place as today.

### Worked example

Tim has connected with 20 people, so Like your picks has 20 points and
each fixed part 20. A BJJ coach in Bonn, about 26, who commented 4 days
ago, with the signals Works with people and Athlete:

| Part | Value | Points |
| --- | --- | --- |
| Who they are | 2 of 3 | 13 |
| Younger | all | 20 |
| Active lately | 91 %, a comment | 18 |
| Near you | 65 % | 13 |
| Like your picks | 70 % | 14 |
| **Fit** | | **79** |

A shop owner in Bielefeld, age not known, whose newest activity is a
like 80 days ago, with no signals beyond owning the business and traits
Tim's picks rarely have, reaches about 17.

## Rejected is Tim's no

Tim rejects a profile with Not this person on the person. Since #67 a
rejected person leaves All and Under 30 for a list of their own,
Rejected, and Undo brings them back. A profile search finds a person by
their own profile, so the profile cannot be someone else's, and in
practice a rejection is Tim saying no to the person.

So the fit learns from it, with no new mark and no reason to give.
Tim, 21 November 2026: he either connects or rejects, and everyone else
is still his to do. A reason is not needed, because the traits of the
people he rejected, set against those he connected with, show what they
have in common. Undo takes the no back.

## How the fit steers the searches

**The order of the due searches.** Searches that never ran still come
first, so each gets one try. After them, `RankSearches` takes the
searches by connections per person found, each with one connection's
worth of doubt, so one lucky find does not put a search on top. Among
searches without connections, those whose open people have the best fit
come first, the mean of their 10 best. A search with 5 people Tim
rejected, and none he connected with, pauses itself, with a line
that says why. Tim resumes it with one click.

**New searches from Tim's picks.** After a run, the app makes up to 2
new searches from the headline words that count most for Like your
picks, said by at least 3 people Tim connected with. Each word goes
with the city near Where you are where most of those people live, like
"BJJ Trainer Bonn". A search that is already there is not made again,
nor one with a word from the exclusions. They show on Searches as
**Made from your picks**, with the trait that made them, and run first
in the next run, after those Tim added. At most 10 of them wait to run
at once. Tim pauses one with one click. Each costs one call to Exa
against the month's budget, like any search.

**Exclusions suggested, never added.** A headline word found on at
least 5 people Tim rejected, and on nobody he connected with,
shows as a suggestion beside the exclusions. Tim adds it with one click. The app
never adds one itself, because an exclusion hides people.

**Later, to try first.** Exa can find pages similar to a link. Whether
its index finds people like a person Tim connected with this way, with
`livecrawl` never and only LinkedIn profiles in the answer, is checked
on Tim's Mac before anything is built on it.

**Later, in words.** The local language model that writes drafts could
read the headlines of the people Tim connected with and those he
rejected, and say in a sentence what sets them apart, as a suggestion
for new searches and exclusions. It adds nothing to the counting, which
already finds those traits, and a draft already takes a while on the
Mac. It waits until the counting has been tried.

## How we know it works

When Tim connects with someone, the app keeps their fit and their place
in the order of open people at that moment. Searches shows one line:
"Of your last 20 connections, 13 were in the top 20 by fit." The aim
from [search-strategy.md](search-strategy.md) stands: at least half. If
the line stays below that after 40 connections, the weights are
revisited with the counts per part.

## Where it lives

- The fixed parts are plain Go functions in the rubric,
  `internal/rubric/fit.go`, beside the signals they read, so no new part
  is needed. The distances are `place.Km`, from the centres of the
  cities the app knows, as GeoNames gives them. The store gives each
  person's facts and Where you are, `Data.Fitter`. The traits of Like
  your picks will sit beside them.
- The fit is worked out when a list is asked for, never kept, because it
  depends on today's date, Where you are and every choice Tim made. The
  traits of everyone are counted once per list. On a file of 5000
  people that is about 150,000 counts, a few milliseconds, to be
  measured in the pull request. Nothing slow happens inside
  `Store.Update`.
- New in the file: the setting Where you are, `home_city`, and later the
  fit and place at connecting. What the profile text says, the newest
  post or comment so far, is read from the text whenever it is set or
  the file is read, never kept beside it.
- No language model, no paid service, no request to LinkedIn. Only
  counting what the file holds.
- The app's log never names a person or a word from their headline.

## Order of work

One pull request each, as in the issue's Progress list:

1. The fit from the four fixed parts, Where you are in Settings, People
   sorted by fit by default, the parts under Why.
2. Like your picks, learning from connected and rejected people.
3. Searches ordered by fit, new searches made from Tim's picks, and
   exclusions suggested.
4. The fit and place at connecting, and the line on Searches.

## Not part of this

- The fit never hides anyone. It only orders. The rules for who is
  kept stay as they are.
- A local language model reading each profile for fit. A draft already
  takes a while on the Mac, doing it for every person found would make
  runs far longer than Tim wants to watch.
