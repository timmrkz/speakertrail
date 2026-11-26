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
2. A nickname between the first and the family name, in quotes or
   brackets or after "aka", is the name a greeting takes: "Katharina
   „Kati“ Vogt" is Kati. Other brackets, like pronouns, "(FH)", a role
   or a maiden name at the end, are left out.
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
   Mara". Words of a tagline count only joined to a word of the name, so
   "Mara Beispiel | Yoga Köln" at `yoga-koeln` stays Mara. Where the name holds no first name, like "Dr. Beispiel", the
   word before the family name in the link is taken, like
   `annette-beispiel`.
8. A name in capitals or small letters only is greeted "Mara", Turkish
   ones in Turkish case, "İbrahim", "Işıl".

Where nothing holds a first name for sure, the draft writes `…` for Tim
to fill in, and the prompt gets the whole name.

## How well it reads

Measured on 25 November 2026 on invented names, since real ones never go
into the repository, all in `internal/names/testdata`.

- `realistic.tsv`: 265 names in the shares one meets on profiles of
  self-employed people in NRW, more than half a plain "First Last",
  each with the link its profile would have. It was written blind, by
  someone who had not seen the rules, and measured once, as it came.
  This is the number to go by.
- `blind.tsv`: 326 hard names, also written blind, 145 of them without
  a link. The rules were improved where it showed a kind of name read
  wrong, so its second number is no longer blind.
- `names.tsv`: 139 names written with the rules in mind, "Yoga mit
  Mara" only in a comment, since it is read wrong.

The libraries were run as they come, on the name alone, since none
reads a link, from a scratch module outside the repository. A first
name counts when it is written exactly as the greeting writes it, so
"MARA" for "MARA BEISPIEL" is wrong. Links without a number at their end
were given one afterwards, so no test holds a link a real person might
have.

| Way of reading | realistic.tsv | blind.tsv, first run | blind.tsv now | names.tsv |
| --- | --- | --- | --- | --- |
| the first word, before this change | 210 (79 %) | 148 | 148 | 69 |
| `internal/names` | 256 (97 %) | 264 | 303 | 138 |
| python-nameparser 3.2, the best open library found, Python, LGPL | 228 (86 %) | 220 | | 115 |
| Go: vinser/parse-full-name | 219 (83 %) | 177 | | 86 |
| Go: caffix/fullname_parser | 219 (83 %) | 175 | | 85 |
| Go: polera/gonameparts | 206 (78 %) | 152 | | 67 |
| probablepeople 0.5 | 179 (68 %) | 144 | | 70 |

One of the nine names of `realistic.tsv` read wrong was a mistake,
"Dipl.-Psych. Claudia" greeted "Psych", fixed since, so it reads 257
now. The other eight are marked `# miss` there.

The Go libraries are made for English names, "Mr.", "Jr.", "III", and
know no German titles. python-nameparser knows many, but misses titles
without their dots, styled letters and the link, and would need Python
on the Mac. So none is used.

## What it still reads wrong

The names read wrong today are marked `# miss` in `realistic.tsv`, 8
of 265, and `blind.tsv`, 23 of 326.

- A role or business before the name, like "Hebamme Lotte Brinkmann" or
  "Praxis Dr. Ulrike Sander", where the link does not hold the name. A
  list of first names would catch these, see below.
- A role written into the link as well, "Fotografin Mara Beispiel" at
  `fotografin-mara-beispiel`, since "Anna Lena Beispiel" at
  `anna-lena-beispiel` looks the same.
- Two given names used together, like "Maria Grazia" or "Abdul Rahman",
  are greeted by the first alone.
- A family name first without a comma or capitals, "Krüger Gabriele".
- A name inside a sentence, "Yoga mit Svenja", "Hey, I'm Grace Okoye".
- One word alone, like "Weber", is taken as a first name.

A list of first names with a licence free to ship, like the given names
of Wikidata, CC0, would find the first name among other words. It is
not built yet, because the link already finds most of them, and
nearly every person here has one.
