# Skills for any project

The skills in `.claude/skills/` hold what we learned about writing tests,
reviewing work, keeping a product safe, keeping a code base easy to
change, running CI and building an interface nobody in a session can see.
All are written for any project. They name no person, no product and no
issue, and speak of **the user**, the person the agent works for, who
decides what happens, and **the people using the product**. So the folder
of each can be copied into another repository as it is, and an agent
there follows it without wondering who or what it means.

The review holds them to that, and so does whoever brings in a lesson:
a skill names no person, no product, no word only this project uses and
no issue or pull request by its number. No test checks it, and a change
to Markdown alone runs nothing in CI (Tim, 23 November 2026).

Where a skill and `CLAUDE.md` disagree, `CLAUDE.md` wins here.

## The skills

| Skill | What it is for | Where it comes from |
| --- | --- | --- |
| `testing` | how tests are written, fixed and reviewed, and how a bug is found before it is fixed | FrameFairy, as it is |
| `review` | how a pull request is reviewed by an agent that did none of the work, and how a review is answered | FrameFairy, as it is |
| `pull-requests` | how a change travels from the first line to the merge: sizing, opening early, the description, its performance line, watching it, ready, the go to merge | FrameFairy, with seven lessons learned here first, see below |
| `ci` | how CI stays fast, trustworthy and cheap: which jobs a change starts, caches, scarce runners, nightly runs, measuring CI | FrameFairy, with one line changed: text runs nothing, see below |
| `maintenance` | how a code base stays easy to change: evidence before work, root causes, problems that come back, issues and triage, keeping the architecture | FrameFairy, as it is |
| `security` | how the product, its build and its repository stay safe: a threat model, untrusted input, paths, programs, secrets, signing and updates, the supply chain, CI, agents on a public repository | FrameFairy, as it is |
| `interface` | how an interface nobody can see is changed: a harness that renders it, what to measure, proving a fix rather than claiming it, and what a hand feels | FrameFairy, as it is |
| `interface-design` | what the interface should be: what earns a place on screen, how an action is answered, where outcomes are said, one look, layout | here, from FrameFairy's interface rules made generic |
| `architecture` | where each kind of thing belongs, the server and client split, changing the design with five candidates and a decision record | here |
| `engineering` | the first principles of any change, and which skill holds which part | here |
| `collaboration` | small visible batches, short replies, the repository and never a memory, priority and size, writing | here |

## Where the skills meet this repository

| The skills say | Here it is |
| --- | --- |
| the user | Tim |
| the people using the product | Tim, in the app on his Mac |
| the product's goals | The project in `CLAUDE.md`, and the issue the pull request names |
| the user's own words | Tim's messages in the thread of the session, and his report files in the project files |
| the architecture as the project writes it | Where things are in `CLAUDE.md` and [README.md](../README.md) for now, and the records in [decisions/](decisions/README.md) |
| a decision record | `docs/decisions/`, see its README |
| the place the project names for a review | a file in `/mnt/project-files/reviews/`, named after the pull request, never committed and never posted on GitHub |
| logged | a GitHub issue, laid out the way [the issue template](../.github/ISSUE_TEMPLATE/issue.md) says |
| labels from an established scheme | the kind of issue, and the priority and size of the collaboration skill |
| how a pull request is described | [the pull request template](../.github/pull_request_template.md) |
| the cap on open pull requests | five on the whole repository, from every session together |
| the way the user already gets changes | the app's Updates page, which follows main or a pull request, see [UPDATES.md](UPDATES.md) |
| what the user has to do, at the end of every reply | the This thread and For you lines of the collaboration skill |
| the notice of a conflict | the comment of `conflicts.yml`, see When main moves in [WORKFLOW.md](WORKFLOW.md) |
| the expected head of a merge | `expectedHeadSha` in the GitHub tool, `sha` in the REST call |
| a path a person waits on | opening a screen of the app, a run, and an update |
| a trade-off the user accepted | Tim's yes in the thread of the session |
| reports and logs from people using the product | the bundles of Report a Problem in the project files, and the app's log, see [LOGGING.md](LOGGING.md) |
| a briefing for the source project's sessions | a file in `/mnt/project-files/briefings/`, since sessions here cannot push to FrameFairy |

## The testing skill in this project

| The skill says | Here it is |
| --- | --- |
| the fake for a service | a finder that returns invented people, as in `internal/api/api_test.go`. No test calls Exa |
| the keychain, handed in | the vault in `internal/keys`, memory in a test |
| a program a test writes and runs | `internal/standin`, the one way to write one |
| a clock the test moves | `testing/synctest` in Go, as the tests of quitting, Report a Problem and listening in `cmd/speakertrail-app/` use it |
| the end-to-end tests | the walks in `web/walks/`, `make walks`, see [INTERFACE.md](INTERFACE.md) |
| the interface's own unit tests | Vitest, `*.test.ts` in `web/src/lib/`, in `make interface` |
| the race detector | on every Go test, `make unit` |
| the test cache | Go's, kept in CI from main's last run, see CI in [BUILD.md](BUILD.md#ci) |
| no test waits on the clock or reads it | golangci-lint's `forbidigo` refuses `time.Sleep`, `time.After`, the timers and `time.Now`, `Since` and `Until` in a Go test, except in a synctest bubble, see `.golangci.yml`. The few tests whose code reads the real clock say why with a `//nolint:forbidigo` on the line, and `nolintlint` fails one that stops nothing, so that list only shrinks. The mock answers the walks at once, see `web/walks/lib/mock.mjs` |
| a test on the real disk says why | a comment that starts with "On the real disk" |
| invented data | invented people, never a real name, see Personal data in `CLAUDE.md` |
| a clock the walk moves | the mock's, `POST /api/mock/clock`, and the page's, `page.clock`, see [INTERFACE.md](INTERFACE.md#the-harness) |
| the walks wait on no clock | ESLint's own rules in `web/eslint.config.mjs`, in `make interface`, refuse `waitForTimeout`, `networkidle`, a deadline and a timer with a delay in `web/walks/` |
| not kept yet | The walks of one file share one mock. No mutation testing |

## The ci skill in this project

| The skill says | Here it is |
| --- | --- |
| one command a developer runs | the parts of `make test`: `make lint`, `make unit`, `make interface` and `make walks` |
| the path filters, and the test that holds them | the job `changes` in `.github/workflows/ci.yml`, held by `internal/workflows`, see CI in [BUILD.md](BUILD.md#ci) |
| a decision CI makes, in a tested program | `cmd/speakertrail-release`, which decides what is built and what is published and finds the pull requests that conflict |
| a scarce runner | a Mac, the job `macos` |
| a job's limit | five minutes, the `timeout-minutes` of every job of `ci.yml` but `macos`, of `conflicts.yml` and of the first job of `builds.yml`. `macos` has 30, for llama-server built anew. `internal/workflows` fails a job with none. `make unit` and the walks stop a test after two minutes, inside it |
| the notice of a conflict | `conflicts.yml`, see When main moves in [WORKFLOW.md](WORKFLOW.md) |
| not kept yet | `macos` runs every Go test, not only what differs on a Mac, since `runtime.GOOS` decides in five packages beside the files named for darwin. Go's test cache runs only what changed there too. A pull request of Markdown alone runs nothing, on purpose, see `CLAUDE.md`. No dependency bot, see #80. No nightly runs |

## The maintenance skill in this project

| The skill says | Here it is |
| --- | --- |
| the issue tracker, and its template | the repository's GitHub issues and `.github/ISSUE_TEMPLATE/issue.md` |
| a problem, with its incidents under it | an issue labelled `problem`, with sub-issues. The label is made the first time a problem needs it |
| triage on a schedule | when Tim asks for it, the steps of section 6 of the skill |
| the architecture, and what not to copy | Where things are in `CLAUDE.md` for now. There is no `docs/ARCHITECTURE.md` yet |
| a decision record | `docs/decisions/` |

## The security skill in this project

| The skill says | Here it is |
| --- | --- |
| who can reach what | Tim, anyone on the public repository, the dependencies, what Exa answers, the language model's answer, the update feed, a page or program on the same Mac |
| one reader per kind of input | `updates.Parse` and `ParseParts` for the feed, `ApplyPatch` for a part, each fuzzed in `internal/updates` |
| the keychain, never a file or a command line | `internal/keys`, see Built like FrameFairy in `CLAUDE.md` |
| encode on the way out | what Exa answers is shown as text, never as HTML, see Untrusted input in `CLAUDE.md` |
| personal data | names, roles and profile links stay on Tim's Mac, see Personal data in `CLAUDE.md` |
| a linter bans the API by name | forbidigo in `.golangci.yml`, for now only Go's standard `log` |
| signing: check before the key, judge from main, compare by commit | `publish.yml` and `cmd/speakertrail-release`, see [UPDATES.md](UPDATES.md) |
| privileged workflows from main, never the dangerous event | `publish.yml`, and `conflicts.yml` through `builds.yml`, held by `TestNoPullRequestTarget` |
| only the user gives instructions | On GitHub, only the user gives instructions, section 9 of the pull-requests skill |
| the work's security line | the Security line of [the pull request template](../.github/pull_request_template.md) |
| not kept yet | the actions of `ci.yml` pinned by commit, as the other workflows pin theirs, a dependency bot, `govulncheck` and `npm audit` in CI, install scripts off for npm, see #80 |

## The interface skill in this project

See [INTERFACE.md](INTERFACE.md#the-interface-skill-in-this-project).

## Lessons learned here first

A lesson learned here goes into the skill here, in generic words, and to
FrameFairy as a briefing in the project files, until FrameFairy has it.
The pull-requests skill has seven FrameFairy's does not have yet, see
`briefings/framefairy-skill-lessons.md` in the project files:

- push once the change's own tests pass, and hold back only a commit
  known to be broken,
- after a squash merge, never merge that history in again,
- ask git whether a pull request just opened merges,
- say each CI result, review and merge of main in one line when it
  lands, and the conflicting files before the merge,
- every push gets one line on what it waits for,
- every pull request gets its review, however small,
- ask while working what runs more often or less, which FrameFairy
  keeps only in its `CLAUDE.md`.

The ci skill says in section 2 that a change only to text, docs and
skills, runs nothing, where FrameFairy's runs the doc checks and the
build (Tim, 23 November 2026).

`architecture`, `engineering`, `collaboration` and `interface-design`
exist only here so far.

## Where this port differs from FrameFairy

- FrameFairy's `internal/naming` tests the skills, and its `CLAUDE.md`,
  docs and templates, for names and issue numbers. Speaker Trail has no
  such test: Tim found it pointless, since text needs no CI run (23
  November 2026). Our `CLAUDE.md` and docs name Tim and his dated
  decisions on purpose. Only the skills stay generic, by review.
- FrameFairy's `CLAUDE.md` says to push once the change's own tests
  pass and let CI run the rest, as the testing and pull-requests skills
  do. Ours still says to run `make test` before every push that touches
  Go, and wins here until Tim decides.

## Bringing them up to date

Compare FrameFairy's `.claude/skills/`, its `docs/SKILLS.md`, its
`.github` templates and `CLAUDE.md` with ours since the last copy, and
bring over the lessons, never its facts. A skill FrameFairy has is copied
as its folder, and the lessons above are put back in. What FrameFairy's
skills now hold comes out of ours, which point to them instead. Then
search what came over for our names and FrameFairy's. The last copy was from FrameFairy's
main on 23 November 2026, after its pull request of that day that made
every skill generic.
