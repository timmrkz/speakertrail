# An update downloads only what changed

Issue #40, in one pull request.

## The problem

Tim, 20 November 2026: "please track an issue to make updates smarter in
a way that we're only downloading what actually changed for an update."
And when the work started: "my next question would be, why are we still
downloading so much? Like 7 MB for basically any pull request, while the
only thing that's changing sometimes is a couple hundred or 1,000 lines
of code, and that's not 7 MB. So if you find flaws in the reference
implementation, try to make it better in meaningful ways, so that the app
is only downloading what's actually changing."

FrameFairy took its updates from 67.5 MB to 7.3 MB in its pull request
#923, decision record
`docs/decisions/2026-11-18-an-update-downloads-only-what-changed.md`
there. Speaker Trail's updater is FrameFairy's from before that change.

What an update is on main at dc65c9a, read from the zips on the release
`dev`, 19.9 MB, 52.3 MB unpacked:

| File | unpacked (MB) | in the zip (MB) | changes between builds |
| --- | ---: | ---: | --- |
| `libonnxruntime.dylib` | 28.9 | 7.8 | no |
| `speakertrail-app` | 19.5 | 10.6 | yes, 5.4 MB of it debug info |
| `libsherpa-onnx-c-api.dylib` | 3.5 | 1.3 | no |
| the rest | 0.4 | 0.2 | seldom |

#38 adds `llama-server`, 6.3 MB in the zip, and makes it 26.4 MB.

How small the change to the app program can be, measured once on the
cloud machine. The Mac programs come from the zips on `dev`, stripped with
`llvm-strip` the way `-s -w` strips them. The Linux programs were built
here at each commit with `-s -w -buildvcs=false`, because `dev` keeps only
the newest build of each channel. A size packed the same way is the same
every time, so there is no spread.

| From, to | what changed | the program packed with zstd (MB) | zstd `--patch-from` (MB) | bsdiff (MB) |
| --- | --- | ---: | ---: | ---: |
| Mac, #39's build to main's | nothing in the app, only the version stamped in | 4.72 | 0.00 | 0.002 |
| Linux, 040111d to 893e951 | nothing in the app | 5.31 | 0.00 | 0.000 |
| Linux, 93aeb44 to 040111d | 82 lines of the interface, #30 | 5.31 | 0.24 | 0.072 |
| Linux, 04ae6b2 to e18ea0f | 605 lines of Go and interface, #23 | 4.97 | 0.81 | 0.241 |
| Mac, main's build to #38's | a new feature, thousands of lines | 4.79 | 1.52 | 0.793 |

FrameFairy measured a binary diff of its app program at 6.6 MB against
7.2 MB packed, and dropped diffs for it. That measurement was of programs
that still carried their debug information, which Go writes compressed,
so any change scrambles all of it. Measured on Speaker Trail's program
with its debug information, the diff for #38 is 6.4 MB too. Without it,
bsdiff is 0.8 MB. bsdiff wins over zstd's `--patch-from` because a change
moves every address after it, and bsdiff stores the difference of
matching bytes, mostly zeros, where `--patch-from` needs bytes that are
exactly the same.

## Five simulations

1. **Who may know about whom.** FrameFairy's design as it is. The zip a
   build is published as stores its files, each file is a piece named by
   its SHA-256, and a parts file says the zip in order. The app writes
   the zip from the files it has and the pieces it lacks, and Wails'
   updater checks the signature over the whole zip. The app knows only
   files by their checksums. A usual update is the app program packed,
   about 5 MB here.
2. **What changes together.** What changes between two builds is a few
   bytes of a file, not a file. So besides its pieces, a build publishes
   a patch of each of its files against the same file in every build the
   channel list holds when it is published, main's and each open pull
   request's, made with bsdiff. The parts name each patch by the file it
   starts from. The app downloads the patch when it has that file, the
   piece when not. Cost: the publish job downloads the pieces it makes
   patches from and diffs each pair, a few seconds a pair, and every
   update is checked twice, by the file's checksum and by the signature.
   An app that runs a build no longer listed takes the piece.
3. **Where state lives.** The app keeps every file it ever had by its
   SHA-256, as git and OSTree do, and installs a build from that store
   with an installer of its own. Cost: a second updater beside Wails' to
   keep safe, and the store grows. Patches still need a base, which the
   running app already is.
4. **What runs together.** The files that rarely change, onnxruntime and
   llama-server, leave the update and are fetched once, like the speech
   models. Cost: an app that is no longer whole and signs programs living
   outside it. Simulation 1 already downloads them never twice.
5. **What a test has to build.** Each file is cut into chunks where its
   content says, as casync and restic do, and the app downloads the
   chunks it lacks. Cost: a Go program moves every address after a
   change, so most chunks change, and a chunk is never smaller than the
   exact-match `--patch-from` measured above, 0.24 to 1.52 MB.

## What converges

- **Files by their SHA-256**, in 1, 2, 3 and 5, so the same bytes are
  never downloaded twice.
- **The signature over the whole zip, checked by Wails**, in 1, 2 and 5.
  Only 3 moves it, at the cost of a second updater.
- **The app stays whole**, in all but 4.
- **The difference of a file against the one installed**, in 2 and 5,
  measured smaller in 2.

Dropped: the store of 3, because the running app is the store that
already exists, the split of 4, and the chunks of 5, which measure
larger than a patch.

Kept beside it, because each makes every update smaller: the app program
ships without its debug information and Go's commit stamp, and the
speech libraries lose their local symbols.

## The decision

Simulation 1, ported from FrameFairy as it is, with the patches of 2 on
top, drawn as layers, each using only the ones under it:

```
publish.yml                 check, pack, sign, upload what the release lacks
scripts/bases.sh            names the builds patched against: the listed ones, and the
                            newest that left the list
cmd/speakertrail-release    pack: the stored zip, its parts, its pieces, its patches
                            against the files of those builds, the stored zip packed
                            sign: the zip the build left, and the stored zip as Stored
scripts/unreachable.sh      removes zips, packed zips, parts, pieces and patches nothing
                            reaches, keeping the parts of the builds patched against
cmd/speakertrail-app        hands Source the running app's folder
internal/updates            Parts, Part, Patch, Pack, Split, the assembling Download,
                            Patch applied, the packed zip it falls back to
Wails' pkg/updater          hashes, checks the signature, unpacks, swaps, as before
```

A patch is bsdiff's own format, BSDIFF40, made with
`github.com/gabstv/go-bsdiff` in the release tool, which only CI runs.
The app applies it with code of its own and Go's `compress/bzip2`, so no
new code reads untrusted bytes in the app but ours, which refuses a patch
that writes outside the file it says it makes. A patch is untrusted like
a piece: what it makes is checked against the file's SHA-256 in the
parts, and the zip against its signature. A wrong one costs the piece,
never the update.

A patch is published only when it weighs less than the piece.

The builds patched against are not only the listed ones. A build leaves
the list when a newer commit of its channel replaces it, or when its pull
request is merged, and the app that runs it is the one most likely to
update next: to main, right after a merge. So the parts of the newest
eight builds that left the list stay on the release, with the pieces they
name, and each new build is patched against them too. The other way
round, an app that picks a build older than its own, takes the piece:
that build was published before the app's build existed.

## What follows

- An update whose code did not change downloads a few kilobytes. A usual
  pull request downloads tens to hundreds of kilobytes, a big feature
  under 1 MB. FrameFairy's design alone would download about 5 MB for
  each.
- An app with few of the files, one made on the Mac, gets the packed zip.
  So does an update whose parts fail.
- An app from before this change downloads the deflated zip, as before,
  so the first update after the merge is still the whole app.
- The first build published after the merge has no build with parts to
  patch against, so it is pieces. Every build after has patches.
- Picking a build older than the one running takes the piece, about 5 MB.
- Every publish downloads the pieces it patches against and runs bsdiff
  for each, which makes the publish slower, see the pull request.
- It was wrong if updates still download whole pieces of the app
  program: the app's log says what each update took, kept, patched and
  fetched.
