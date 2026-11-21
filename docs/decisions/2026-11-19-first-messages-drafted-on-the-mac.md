# First messages drafted on the Mac by a local language model

Pull request #38. Written while the pull request was open, after its
first code, which the architecture skill says comes before. The review
of the pull request asked for it, finding 6.

## The problem

Tim, 19 November 2026: for each new connection on LinkedIn he took
screenshots of the profile, pasted them into Claude, pasted the message
back into LinkedIn, changed it and sent it. He wants to write to more
people. His choice: keep the profile data the app already gets from Exa,
because it is what a message is written from, and draft with a local
language model the way FrameFairy does, Gemma 4 26B, downloaded and
managed in Settings like the speech model. A button starts the draft in
the background, the draft shows in a box he can change, a double click
selects all of it, and he opens the profile with the button there is.

Measured on the code as it was: `internal/search` read Exa's text of a
profile, up to 8,000 letters, and kept only the headline, the place, the
roles and the dates. Nothing in the app ran a language model. The speech
model of Report a Problem had a download, a check and a Settings row,
`internal/speech` and `speechmodels.go`. FrameFairy runs llama.cpp's
llama-server as a program of its own, `engine/local.go` and
`modelhost.go`.

## Five simulations

### Who may know about whom

Drafting inside `internal/engine`, as one more kind of work in a run. A
run would then know about models, prompts and people's messages, and
the API about both through one door. A part of its own, `internal/draft`,
knows the store, the language model and Exa's profile, and nothing knows
it but the API and the app's `main`. That is one name crossing each new
boundary: `Drafter` from the API, `Host` and `Ask` from `language`,
`Finder` from `search`.

### What changes together

The model in the app's own process through a cgo binding of llama.cpp,
or llama-server as a program beside it. In the process, a model that
runs out of memory takes the app down with it, and a llama.cpp upgrade
is a Go dependency with Metal built through cgo. As a program it is
FrameFairy's way, its build script, its flags and its release check
entry are copied as they are, and a llama.cpp upgrade is one pin in
`scripts/build-llama.sh`. What changes with llama.cpp changes there and
nowhere else.

### Where state lives

The profile text on the person in the one file, in a file of its own,
or not kept and read from Exa for each draft. Kept on the person, it is
there when a draft wants it and survives what the file survives, at the
cost of a larger file written whole on every change, about 8 KB a
person. A file of its own keeps saves fast but breaks "one JSON file".
Read for each draft, it costs a call to Exa per draft and throws away
what Tim asked to keep. The draft itself and its times are kept on the
person and in `DraftTimes`, the models in the models folder with the
speech models, and a running server in a note of its own, so a crash
leaves no server unknown.

### What runs together

Drafts in the run's queue, beside the checks, or a worker of their own.
A run is short and started by hand, and a draft is asked for one person
at a time while Tim reads. A worker of its own writes one draft at a
time, in the order asked, since one model fits in memory. The model
stayed loaded for ten minutes after a draft, so the next started at once.
The first still waited 7 s for the load on Tim's Mac, so on 21 November
2026 he chose to load it as the app starts and keep it until it quits,
about 16.6 GB for Gemma 4 26B, which Metal keeps for llama-server and no
other program can have while the app is open. Loading it when a person
opens, and keeping it as before, were the other choices.

### What a test has to build

A model cannot run in a test. With llama-server as a program, the test
program stands in for it, started the same way, with the same key and
the same streamed answer, so `Start`, the key and a model that fails to
load are tested on a real process. The drafter takes the host and the
ask as fields, so its tests run with neither. The window is told by the
event `draft`, which a walk sends the way the app does.

## What converges

- llama-server as a program beside the app, as FrameFairy, in four of
  five. Only the cgo binding of the second needs it in the process.
- A part for drafting of its own, `internal/draft`, and one for the
  model, `internal/language`, beside `internal/speech`.
- The download of a model shared by both models, `internal/fetch`, and
  one controller and one Settings row for both, `modeldownloads.go` and
  `ModelRow.svelte`, as FrameFairy's one ModelCard.
- The profile text on the person, in the one file. Its cost to every
  save is measured in the pull request, and whether it moves to a file
  of its own is Tim's to decide. He chose a file of its own on 20
  November 2026, see below.

## The decision

From the top, each layer using only the ones under it:

1. The window: `FirstMessage.svelte` in a person's view, `LanguageRow`
   and `SpeechRow` on `ModelRow` in Settings.
2. The API: `/api/drafts`, `/api/people/{id}/draft`, and
   `/api/language` answered by the app, `languagemodels.go` on
   `modeldownloads.go`.
3. `internal/draft`: one draft at a time, its step and its time left.
4. `internal/language`, `internal/search` and `internal/store`: the
   model and llama-server, Exa's profile, the file, and the profile
   texts in `profile-texts.json` beside it, by profile link.
5. `internal/fetch`: a download that resumes and checks.

The models live in the app's own folder. A model FrameFairy has whole in
`~/.framefairy/models` is linked from there rather than downloaded.

## What follows

It costs a program of 17 MB more in every build, a llama.cpp build of a
few minutes when its script changes, cached after, and a second file.
Measured with 1,000 people of 8,000 characters each: a save takes about
8 ms, as before the texts, where the texts in the one file took about
82 ms. A save that brings a new text, a check that finds someone or a
profile read for a draft, still writes all the texts, about 74 ms. It buys first messages written on the Mac, with
no paid model and nothing of a profile sent anywhere but Exa.

It was wrong if drafts are too slow to wait for on Tim's Mac, if the
model writes messages Tim rewrites whole, or if saves of the file become
something Tim notices.
