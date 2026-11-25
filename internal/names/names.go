// Package names finds the parts of a person's name as LinkedIn and Exa
// show it, where titles, degrees, pronouns, emoji and a tagline can stand
// beside it, like "Dr. med. Mara Beispiel", "Mara Beispiel, MBA
// (sie/ihr)" or "Beispiel, Mara". It is the one place that says what a
// first name is.
//
// It reads the name the way a person would: titles before it, degrees
// after it, and the order the profile's link gives, since LinkedIn makes
// the link from the first and the family name, like
// linkedin.com/in/mara-beispiel-4b1a2c93. No list of first names is
// needed, so a name never seen before is read as well as a common one.
// See docs/names.md for how well it reads the forms profiles use.
package names

import (
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// First is the first name, to greet someone by, in the case a greeting
// writes it, "Mara" of "MARA BEISPIEL". A nickname they show, like
// "Mimi" in "Mara (Mimi) Beispiel", is the name they go by. The link is
// their profile's, or empty. It is empty when neither the name nor the
// link holds a first name for sure, like "Dr. Beispiel", which is a
// title and a family name.
func First(name, link string) string {
	n := read(name)
	if n.nick != "" {
		return cased(n.nick)
	}
	first := ""
	switch {
	case n.given != "":
		first = n.given
	case len(n.words) > 1 || len(n.words) == 1 && !n.titled && !n.initialed:
		first = n.words[0]
	}
	if w := byLink(n.all, link); w != "" {
		first = w
	}
	if first == "" || !latin(first) {
		// A link can hold the first name the name leaves out, like
		// annette-beispiel for "Dr. Beispiel", or writes in Latin letters.
		if w := fromLink(n, link); w != "" {
			first = w
		}
	}
	return cased(first)
}

// Family is the last word of the family name, "Beispiel" of "Mara von
// Beispiel". It is empty when the name has one word and no title before
// it, since that word is more likely a first name.
func Family(name string) string {
	n := read(name)
	switch {
	case n.family != "":
		return n.family
	case len(n.words) >= 2 || (n.titled || n.initialed) && len(n.words) == 1:
		return n.words[len(n.words)-1]
	}
	return ""
}

// parsed is a name as read: its own words in the order written, every
// word with letters in all of it, for the link to order, whether a title
// or only initials stood before them, a nickname, and the given and
// family name where the name says which is which, as "Beispiel, Mara"
// does.
type parsed struct {
	words, all          []string
	titled, initialed   bool
	nick, given, family string
}

// read reads a name.
func read(name string) parsed {
	name = plain(name)
	var n parsed
	name, n.nick = nickname(name)
	n.all = strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '-' && r != '\'' })
	// What follows a bar, a slash or a colon is a tagline, or the name in
	// other letters, like "Олена Бондар | Olena Bondar", which a greeting
	// in Latin letters takes.
	segments := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune("|/:", r) })
	if len(segments) == 0 {
		return n
	}
	name = segments[0]
	if !latin(name) {
		for _, s := range segments[1:] {
			if latin(s) {
				name = s
				break
			}
		}
	}
	before, after, comma := strings.Cut(name, ",")
	n.words, n.titled, n.initialed = wordsOf(before)
	switch {
	case comma && familyOnly(n.words):
		// "Beispiel, Mara" is the family name, then the given name. After
		// a comma there are degrees most often, like "Mara Beispiel,
		// MBA", so only a family name before it, like "Beispiel" or "von
		// Beispiel", and a name after it read that way.
		given, _, initialed := wordsOf(after)
		family := n.words[len(n.words)-1]
		switch {
		case len(given) > 0 && named(given, family):
			n.given, n.family = given[0], family
			n.words = append(given, n.words...)
		case len(given) == 0 && initialed:
			// "Beispiel, M." holds no first name.
			n.initialed, n.family = true, family
		}
	case len(n.words) >= 2 && capitals(n.words[0]) && !capitals(n.words[1]):
		// "BEISPIEL Mara" writes the family name first, in capitals.
		n.given, n.family = n.words[1], n.words[0]
	}
	return n
}

// familyOnly says whether the words before a comma are a family name
// alone, one word with particles, like "von Beispiel" or "Van Dijk".
func familyOnly(words []string) bool {
	others := 0
	for _, w := range words {
		if !particles[strings.ToLower(w)] {
			others++
		}
	}
	return len(words) > 0 && others == 1
}

// named says whether the words after a comma are a given name, not
// degrees or credentials. Those are written in capitals, like "ICF ACC",
// so a word in capitals is a name only where the family name is in
// capitals too, as in "BEISPIEL, MARA".
func named(given []string, family string) bool {
	for _, w := range given {
		if degree(w) || capitals(w) && !capitals(family) {
			return false
		}
	}
	return true
}

// capitals says whether a word of more than one letter is written in
// capitals only.
func capitals(w string) bool {
	return len([]rune(w)) > 1 && w == strings.ToUpper(w) && w != strings.ToLower(w)
}

// wordsOf finds the words of a name, whether a title stood before them,
// and whether only initials did, like "M." in "M. Beispiel".
func wordsOf(name string) (out []string, titled, initialed bool) {
	lower := name == strings.ToLower(name)
	fields := strings.Fields(split(name))
	for i, w := range fields {
		w = strings.TrimFunc(w, edge)
		lettered := strings.IndexFunc(w, unicode.IsLetter) >= 0
		switch {
		case !lettered && len(out) == 0:
			// An emoji before the name.
			continue
		case !lettered || strings.HasPrefix(w, "#") || strings.HasPrefix(w, "@"):
			// A dash, a dot, an emoji or a hashtag after it ends the name.
			return trim(out), titled, initialed
		case len(out) > 0 && !lower && small(w) && !particles[w]:
			// A word in small letters after it, like "für" or "geb.",
			// starts a tagline or a maiden name. "von" and "de" are part
			// of the family name.
			return trim(out), titled, initialed
		case len(out) == 0 && initial(w):
			initialed = true
			continue
		case len(out) == 0 && (title(w, titled) || diplom(fields, i)):
			titled = true
			continue
		}
		out = append(out, w)
	}
	return trim(out), titled, initialed
}

// initial says whether a word is an initial, like "M." or "H.-J.".
func initial(w string) bool {
	letters := 0
	for _, r := range w {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return strings.HasSuffix(w, ".") && letters <= 2 && !titles[strings.ToLower(strings.TrimSuffix(w, "."))]
}

// diplom says whether the word at i is the subject of a diploma, like
// "Betriebswirtin" in "Dipl. Betriebswirtin Tanja Röder": it follows
// "Dipl." and two words of the name follow it.
func diplom(fields []string, i int) bool {
	if i == 0 || i+2 >= len(fields) {
		return false
	}
	before := strings.ToLower(strings.TrimSuffix(fields[i-1], "."))
	return before == "dipl" && !small(fields[i])
}

// split puts a space after the dot of a title written onto the name, so
// "Dr.Mara" reads as "Dr. Mara". "Dr.-Ing" and "Mag.a" stay one word.
func split(name string) string {
	r := []rune(name)
	var b strings.Builder
	for i, c := range r {
		b.WriteRune(c)
		if c == '.' && i+1 < len(r) && unicode.IsUpper(r[i+1]) {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// edge says whether a character at the edge of a word is no part of the
// name, like an emoji or a sparkle written onto it. A dot and a hyphen
// stay, since they mark titles and initials.
func edge(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '.' && r != '-' && r != '#' && r != '@'
}

// plain writes a name in plain letters: bold, italic, script and wide
// letters, and small capitals, as the letters they style, emoji and
// other symbols as a dot between words, so "Mara🌿Beispiel" is two
// words, and spaces nobody sees as spaces.
func plain(name string) string {
	var b strings.Builder
	for _, r := range norm.NFKC.String(name) {
		switch {
		case smallCapitals[r] != 0:
			b.WriteRune(smallCapitals[r])
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' || unicode.Is(unicode.Variation_Selector, r):
			// Joiners and selectors inside an emoji, and a space of no
			// width.
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || unicode.Is(unicode.Cs, r) || unicode.Is(unicode.Co, r):
			b.WriteString(" · ")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// smallCapitals are the small capital letters some write a name in, like
// "ᴋᴀᴛʜᴀʀɪɴᴀ", which no normal form makes plain.
var smallCapitals = map[rune]rune{
	'ᴀ': 'a', 'ʙ': 'b', 'ᴄ': 'c', 'ᴅ': 'd', 'ᴇ': 'e', 'ꜰ': 'f', 'ɢ': 'g',
	'ʜ': 'h', 'ɪ': 'i', 'ᴊ': 'j', 'ᴋ': 'k', 'ʟ': 'l', 'ᴍ': 'm', 'ɴ': 'n',
	'ᴏ': 'o', 'ᴘ': 'p', 'ǫ': 'q', 'ʀ': 'r', 'ꜱ': 's', 'ᴛ': 't', 'ᴜ': 'u',
	'ᴠ': 'v', 'ᴡ': 'w', 'ʏ': 'y', 'ᴢ': 'z',
}

// nickname takes the nickname out of a name: one word in quotes, like
// "Mara „Mimi“ Beispiel", in brackets, like "Mara (Mimi) Beispiel", or
// after "aka". What else stands in brackets, pronouns, a maiden name or
// "(FH)", is taken out too, and everything after a bracket that is never
// closed.
func nickname(name string) (rest, nick string) {
	var b strings.Builder
	fields := strings.Fields(name)
	for i := 0; i < len(fields); i++ {
		w := fields[i]
		switch {
		case strings.EqualFold(w, "aka") && i+1 < len(fields) && nickable(fields[i+1]):
			nick = cmpOr(nick, fields[i+1])
			i++
			continue
		case quotes(w):
			if inner := strings.TrimFunc(w, isQuote); nickable(inner) {
				nick = cmpOr(nick, inner)
			}
			continue
		}
		b.WriteString(w)
		b.WriteByte(' ')
	}
	name = b.String()
	for {
		i := strings.IndexAny(name, "([")
		if i < 0 {
			return name, nick
		}
		j := strings.IndexAny(name[i:], ")]")
		if j < 0 {
			return name[:i], nick
		}
		// A nickname stands inside the name, "Mara (Mimi) Beispiel". A
		// word in brackets at its end is more often a role, "Mara
		// Beispiel (Yogalehrerin)".
		inner := strings.TrimSpace(name[i+1 : i+j])
		if nickable(inner) && strings.IndexFunc(name[i+j:], unicode.IsLetter) >= 0 {
			nick = cmpOr(nick, inner)
		}
		name = name[:i] + " " + name[i+j+1:]
	}
}

// nickable says whether a word can be a nickname: one word with a
// capital first, not in capitals only like "(FH)", and no title or
// degree.
func nickable(w string) bool {
	r := []rune(w)
	if len(r) < 2 || !unicode.IsUpper(r[0]) || capitals(w) || degree(w) || title(w, true) {
		return false
	}
	for _, c := range r {
		if !unicode.IsLetter(c) && c != '-' {
			return false
		}
	}
	return true
}

// quotes says whether a word stands in quotes.
func quotes(w string) bool {
	r := []rune(w)
	return len(r) > 2 && isQuote(r[0]) && isQuote(r[len(r)-1])
}

func isQuote(r rune) bool {
	return strings.ContainsRune(`"“”„«»'‘’‚`, r)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// latin says whether a word holds a Latin letter, the script a greeting
// here is written in.
func latin(w string) bool {
	return strings.IndexFunc(w, func(r rune) bool { return unicode.Is(unicode.Latin, r) }) >= 0
}

// trim takes the degrees and initials off the end of the name, but never
// its first word.
func trim(w []string) []string {
	for len(w) > 1 && degree(w[len(w)-1]) {
		w = w[:len(w)-1]
	}
	return w
}

// byLink is the first name as the profile's link orders the name: the
// word whose letters stand first in the link, joined to another word of
// the name, as "marabeispiel" joins Mara and Beispiel. Two words joined
// make sure the link is made from the name, not a business or a slogan
// that holds one of its words. It is empty when the link says nothing.
func byLink(words []string, link string) string {
	handle := strings.Join(pieces(link), "")
	if handle == "" {
		return ""
	}
	best, at := "", len(handle)
	for i, a := range words {
		for j, b := range words {
			if i == j || len([]rune(fold(a))) < 2 || title(a, true) || degree(a) {
				continue
			}
			if k := strings.Index(handle, fold(a)+fold(b)); k >= 0 && k < at {
				best, at = a, k
			}
		}
	}
	return best
}

// fromLink is the first name the link holds where the name has none, or
// none in Latin letters: the word before the family name, like
// "annette" in annette-beispiel-3d4e for "Dr. Beispiel", or else the
// link's first word, like "olena" in olena-bondar for "Олена Бондар".
func fromLink(n parsed, link string) string {
	p := pieces(link)
	usable := func(w string) bool {
		// Two letters are more often initials, like "hj" of
		// hj-beispiel, than a name.
		return len([]rune(w)) >= 3 && !title(w, true) && !degree(w)
	}
	if len(n.words) > 0 {
		family := fold(n.words[len(n.words)-1])
		if n.family != "" {
			family = fold(n.family)
		}
		for k := 1; k < len(p); k++ {
			if p[k] == family && usable(p[k-1]) && p[k-1] != family {
				return raw(link, k-1)
			}
		}
	}
	if len(n.words) > 0 && !latin(n.words[0]) && len(p) >= 2 && usable(p[0]) {
		return raw(link, 0)
	}
	return ""
}

// pieces are the words of a link's handle, folded, like "mara",
// "beispiel" and "ba" of mara-beispiel-4b1a2c93. Digits make a word of
// their own, so "4b1a2c93" falls apart into letters nobody reads as a
// name.
func pieces(link string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(handleOf(link), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsMark(r) }) {
		if f := fold(w); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// raw is the k-th word of a link's handle as it is written there, with
// its umlauts, like "jörg" of jörg-probe.
func raw(link string, k int) string {
	words := strings.FieldsFunc(handleOf(link), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsMark(r) })
	if k < len(words) {
		return norm.NFC.String(words[k])
	}
	return ""
}

// handleOf is the part of a profile's link after /in/, like
// "mara-beispiel-4b1a2c93", with its escapes read.
func handleOf(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	_, handle, ok := strings.Cut(strings.Trim(u.Path, "/"), "in/")
	if !ok || strings.Contains(handle, "/") {
		return ""
	}
	return handle
}

// fold writes a name the way a link may, in small letters, without
// accents, hyphens or digits, and with "ae", "oe" and "ue" as the
// umlauts they stand for, so Jörg, Joerg and jorg are the same.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case r == 'ß':
			b.WriteString("ss")
		case unicode.IsLetter(r):
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("ae", "a", "oe", "o", "ue", "u").Replace(b.String())
}

// cased writes a name in capitals or small letters only, like "MARA" or
// "mara", the way a greeting does, "Mara", and "ANNA-LENA" as
// "Anna-Lena". A name in mixed case, like "McKenzie", stays as it is.
func cased(w string) string {
	if w != strings.ToUpper(w) && w != strings.ToLower(w) {
		return w
	}
	// Turkish writes the capital of i as İ and the small letter of I as
	// ı, so "IŞIL" is "Işıl" and "İBRAHİM" is "İbrahim".
	var c unicode.SpecialCase
	if strings.ContainsAny(w, "İıŞşĞğ") {
		c = unicode.TurkishCase
	}
	parts := strings.Split(strings.ToLowerSpecial(c, w), "-")
	for i, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			r[0] = c.ToUpper(r[0])
		}
		parts[i] = string(r)
	}
	return strings.Join(parts, "-")
}

// small says whether a word starts with a small letter.
func small(w string) bool {
	r := []rune(w)
	return len(r) > 0 && unicode.IsLower(r[0])
}

// particles are the small words that are part of a family name, like
// "von" in "Mara von Beispiel" and "al" in "Fatma al Beispiel".
var particles = set("von", "vom", "van", "der", "den", "de", "del", "della",
	"di", "da", "das", "dos", "du", "des", "le", "la", "zu", "zum", "zur",
	"ten", "ter", "op", "al", "el", "bin", "ibn", "y")

// titles are the titles that stand before a name, in small letters and
// without their dots, so "Dr." and "Dr" are the same. "Mag.a" is "mag".
var titles = set("dr", "drs", "dres", "prof", "professor", "professorin",
	"doktor", "dipl", "mag", "pd", "priv", "univ", "apl", "habil", "herr",
	"frau", "mr", "mrs", "ms", "sir", "dott", "dott.ssa", "mgr", "inż", "inz",
	"lic", "eidg")

// fields are the words that say a doctor's field, like "med" in "Dr.
// med", or a degree's, like "Ing" in "Dr.-Ing". They are titles only
// after another, since "Phil", "Nat" and "Ing" are first names too.
var fields = set("med", "dent", "vet", "rer", "nat", "pol", "oec", "phil",
	"jur", "theol", "soc", "sc", "hc", "mult", "ing", "univ", "medic", "psych",
	"iur")

// professions are the letters of a profession written before a name, as
// they are spelt, like "RA", a lawyer, "StB", a tax adviser, "DI", an
// Austrian engineer, and "HP", a Heilpraktiker.
var professions = set("RA", "RAin", "StB", "WP", "DI", "HP")

// degrees are the degrees and certificates written after a name without
// dots, as they are spelt, so "Ma", a family name, is not "MA". A word
// that ends with a dot is one too, like "M.Sc.", "LL.M." or an initial.
// Before a name, they are titles.
var degrees = set("MBA", "EMBA", "DBA", "PhD", "PHD", "MSc", "BSc", "MA",
	"BA", "MD", "LLM", "LLB", "MPH", "MEng", "BEng", "MAS", "CFA", "CPA",
	"ACC", "PCC", "MCC", "ICF", "Jr", "Sr")

// title says whether a word before a name is a title, given whether one
// stood before it. A word that ends with a dot is one, like "med." in
// "Dr. med.", and so is an initial, so "J. Mara Beispiel" is greeted as
// Mara. A word joined by hyphens is one only when each of its parts is,
// like "Dr.-Ing" and "Dipl.-Kfm.", so "Hans-J." and "Ing-Marie" are
// names.
func title(w string, titled bool) bool {
	if parts := strings.Split(w, "-"); len(parts) > 1 {
		for _, p := range parts {
			if p == "" || !title(p, true) {
				return false
			}
		}
		return true
	}
	if strings.HasSuffix(w, ".") || professions[w] || degrees[w] {
		return true
	}
	first, _, _ := strings.Cut(strings.ToLower(w), ".")
	return titles[first] || titled && fields[first]
}

// degree says whether a word after a name is a degree or an initial.
func degree(w string) bool {
	return strings.HasSuffix(w, ".") || degrees[strings.ReplaceAll(w, ".", "")]
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
