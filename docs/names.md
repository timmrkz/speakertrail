# First names

A first message greets a person by their first name, "Hallo Mara,". The
name comes from their LinkedIn profile as they wrote it, and people write
far more there than their name: "Dr. med. Mara Beispiel", "Mara
Beispiel, MBA (sie/ihr)", "𝗠𝗮𝗿𝗮 𝗕𝗲𝗶𝘀𝗽𝗶𝗲𝗹 🚀 Coach", "BEISPIEL, Mara".
`internal/names` is the one place that reads it. The draft, the prompt's
`{{first_name}}` and the fit rubric's company named after someone use it.

## How it reads a name

1. Bold, italic, script, wide and small capital letters are plain
   letters. Emoji and other symbols part words.
2. A nickname inside the name, in quotes or brackets or after "aka", is
   the name a greeting takes: "Katharina „Kati“ Vogt" is Kati. Other
   brackets, like pronouns, "(FH)" or a role at the end, are left out.
3. What follows a bar, a slash or a colon is a tagline. Where the name
   is in other letters, like Cyrillic, and the tagline holds it in Latin
   letters, the greeting takes those.
4. Titles before the name are skipped: Dr., Dr. med., Prof., Dipl.-Ing.,
   Mag.a, RA, StB, Dott., mgr inż. and the like, with or without dots and
   spaces. A word that only follows a title, like "med" or "Ing", is
   one, so "Phil" and "Ing" stay first names elsewhere. Initials are
   skipped too.
5. Degrees and credentials after the name are left out: MBA, M.Sc.,
   PhD, ICF ACC. A word in small letters, like "für" or "geb.", starts
   a tagline. "von", "van" and "de" belong to the family name.
6. "Beispiel, Mara" and "BEISPIEL Mara" write the family name first.
7. The profile's link decides last. LinkedIn makes it from the first
   and the family name, like `mara-beispiel-4b1a2c93`, so where two
   words of the name stand joined in it, the first of them is the first
   name. That finds Mara in "Coach Mara Beispiel" and in "Beispiel
   Mara". Where the name holds no first name, like "Dr. Beispiel", the
   word before the family name in the link is taken, like
   `annette-beispiel`.
8. A name in capitals or small letters only is greeted "Mara", Turkish
   ones in Turkish case, "İbrahim", "Işıl".

Where nothing holds a first name for sure, the draft writes `…` for Tim
to fill in, and the prompt gets the whole name.

## How well it reads

Measured on 25 November 2026 on invented names, since real ones never go
into the repository. `testdata/names.tsv` holds 139 names written with
the rules in mind, "Yoga mit Mara" only in a comment, since it is read
wrong. `testdata/blind.tsv` holds 326 hard ones, written by
someone who had not seen the rules, to measure them. The libraries were
run as they come, on the first name alone, without the link.

| Way of reading | names.tsv | blind.tsv, first run | blind.tsv now |
| --- | --- | --- | --- |
| the first word, before this change | 69 | 148 | 148 |
| `internal/names` | 138 | 264 | 303 |
| python-nameparser 3.2, the best open library found, Python, LGPL | 118 | 229 | |
| probablepeople 0.5 | 73 | 152 | |
| Go: vinser/parse-full-name | 86 | 177 | |
| Go: caffix/fullname_parser | 85 | 175 | |
| Go: polera/gonameparts | 67 | 152 | |

"First run" is the rules as they stood before the blind set was read.
The rules were then improved where it showed a kind of name read wrong,
so "now" is no longer blind.

The Go libraries are made for English names, "Mr.", "Jr.", "III", and
know no German titles. python-nameparser knows many, but misses titles
without their dots, styled letters and the link, and would need Python
on the Mac. So none is used.

## What it still reads wrong

The 23 names of `blind.tsv` read wrong today are marked `# miss` there.

- A role or business before the name, like "Hebamme Lotte Brinkmann" or
  "Praxis Dr. Ulrike Sander", where the link does not hold the name. A
  list of first names would catch these, see below.
- Two given names used together, like "Maria Grazia" or "Abdul Rahman",
  are greeted by the first alone.
- A family name first without a comma or capitals, "Krüger Gabriele".
- A name inside a sentence, "Yoga mit Svenja", "Hey, I'm Grace Okoye".
- One word alone, like "Weber", is taken as a first name.

A list of first names with a licence free to ship, like the given names
of Wikidata, CC0, would find the first name among other words. It is
not built yet, because the link already finds most of them, since
every person here has one.
