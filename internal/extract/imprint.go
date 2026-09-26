package extract

import (
	"net/url"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Imprint is what a company's imprint, its Impressum, says about who runs
// it. German law (§5 DDG) asks every business website for one, with the
// people who represent the company. Only names are taken from it, never an
// email address or a phone number.
type Imprint struct {
	// Company is the name with its legal form, "Beispiel GmbH".
	Company   string
	LegalForm string
	Postcode  string
	City      string
	// Directors are the managing directors or owners, full names only.
	Directors []string
	// Label is the imprint's own word for them, "Geschäftsführer".
	Label string
}

// Link is a link on a page, with its text or the alt text of its image.
type Link struct {
	URL  string
	Text string
	// Chrome is true for links in a page's header, navigation or footer.
	Chrome bool
}

// Links lists a page's links as absolute addresses.
func Links(body, base string) []Link {
	doc, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	var out []Link
	var walk func(n *nethtml.Node, chrome bool)
	walk = func(n *nethtml.Node, chrome bool) {
		if n.Type == nethtml.ElementNode {
			switch n.DataAtom {
			case atom.Nav, atom.Header, atom.Footer:
				chrome = true
			case atom.Script, atom.Style, atom.Noscript, atom.Template:
				return
			case atom.A:
				if u := absURL(base, attr(n, "href")); u != "" {
					out = append(out, Link{URL: u, Text: cleanText(linkText(n)), Chrome: chrome})
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, chrome)
		}
	}
	walk(doc, false)
	return out
}

func attr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// linkText is a link's text, or the alt or title of an image in it, as
// portfolios often link a logo.
func linkText(n *nethtml.Node) string {
	var b strings.Builder
	var alt string
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		switch {
		case n.Type == nethtml.TextNode:
			b.WriteString(n.Data)
			b.WriteByte(' ')
		case n.Type == nethtml.ElementNode && n.DataAtom == atom.Img && alt == "":
			alt = attr(n, "alt")
			if alt == "" {
				alt = attr(n, "title")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if t := strings.TrimSpace(b.String()); t != "" {
		return t
	}
	if alt == "" {
		alt = attr(n, "title")
	}
	if alt == "" {
		alt = attr(n, "aria-label")
	}
	return alt
}

var (
	imprintText = regexp.MustCompile(`(?i)^(?:impressum|imprint|legal notice|legal|legal info|rechtliches|anbieterkennzeichnung|impressum\s*(?:&|und|/)\s*datenschutz)$`)
	imprintPath = regexp.MustCompile(`(?i)/(?:[a-z]{2}/)?(?:impressum|imprint|legal-notice|legal|site-notice)(?:\.html?|\.php|/)?$`)
)

// ImprintLink finds the link to the imprint on a website's page, on the same
// website. It returns "" when there is none.
func ImprintLink(body, base string) string {
	home := hostOf(base)
	var byPath string
	for _, l := range Links(body, base) {
		if hostOf(l.URL) != home {
			continue
		}
		u, err := url.Parse(l.URL)
		if err != nil {
			continue
		}
		if imprintText.MatchString(strings.TrimSpace(l.Text)) {
			return l.URL
		}
		if byPath == "" && imprintPath.MatchString(u.Path) {
			byPath = l.URL
		}
	}
	return byPath
}

var (
	// directorLabel opens the line that names who represents the company.
	directorLabel = regexp.MustCompile(`(?i)^\s*(?:vertretungsberechtigte[rn]?\s+)?(geschäftsführer(?:in|innen)?|geschäftsführung|geschäftsführende[rn]?\s+gesellschafter(?:in|innen)?|vertreten\s+durch|vertretungsberechtigt|managing\s+directors?|represented\s+by|inhaber(?:in)?|owner|vorstand|ceo)\b(?:\s*(?:\([^)]*\)|/\s*-?in(?:nen)?|\*in(?:nen)?))?(?:\s*:\s*|\s*$|\s+)(.*)$`)
	// roleInFront drops "die Geschäftsführer" after "Vertreten durch".
	roleInFront = regexp.MustCompile(`(?i)^(?:(?:die|den|der|the|ihre[n]?|seine[n]?)\s+)?(?:geschäftsführer(?:in|innen)?|geschäftsführung|gesellschafter(?:in|innen)?|vorstand|managing\s+directors?|ceo)\s*:?\s*`)
	roleAfter   = regexp.MustCompile(`\s*\([^()]{1,40}\)\s*$`)
	directorSep = regexp.MustCompile(`(?i)\s*(?:,|;|\s+und\s+|\s+and\s+|\s+&\s+|\s+sowie\s+|\s+\|\s+)\s*`)
	legalForm   = regexp.MustCompile(`(?:^|\s)(GmbH\s*&\s*Co\.\s*KG|gGmbH|GmbH|UG\s*\(haftungsbeschränkt\)|UG|AG|SE|e\.\s?V\.|e\.\s?K\.|KG|OHG|GbR|PartG|Inc\.|Ltd\.|B\.V\.)(?:[\s,.]|$)`)
	postcodeRe  = regexp.MustCompile(`(?:^|[\s,]|D-)(\d{5})\s+([A-ZÄÖÜ][\p{L}.\-]+(?:[ \-](?:an der|am|im|in|a\.|[A-ZÄÖÜ(][\p{L}.()\-]*))*)`)
)

// ParseImprint reads an imprint's visible text.
func ParseImprint(text string) Imprint {
	var im Imprint
	lines := strings.Split(text, "\n")
	seen := map[string]bool{}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if im.Company == "" {
			if m := legalForm.FindStringSubmatchIndex(line); m != nil && len(line) <= 90 && !directorLabel.MatchString(line) {
				im.Company = strings.Trim(cleanText(line[:m[3]]), " ,")
				im.LegalForm = strings.Join(strings.Fields(line[m[2]:m[3]]), " ")
			}
		}
		if im.Postcode == "" {
			if m := postcodeRe.FindStringSubmatch(line); m != nil {
				im.Postcode, im.City = m[1], strings.TrimSpace(m[2])
			}
		}
		m := directorLabel.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := strings.TrimSpace(m[2])
		// A label on a line of its own names the people on the next lines.
		var names []string
		if rest != "" {
			names = directorNames(rest)
		} else {
			for j := i + 1; j < len(lines) && j <= i+4; j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					if len(names) > 0 {
						break
					}
					continue
				}
				found := directorNames(next)
				if len(found) == 0 {
					break
				}
				names = append(names, found...)
				i = j
			}
		}
		for _, n := range names {
			if k := NormaliseName(n); !seen[k] {
				seen[k] = true
				im.Directors = append(im.Directors, n)
				if im.Label == "" {
					im.Label = labelWord(m[1])
				}
			}
		}
	}
	return im
}

// directorNames reads "Die Geschäftsführer Lena Beispiel (CEO) und Tom
// Test" as two names. A part that is not a full name ends the list.
func directorNames(s string) []string {
	s = roleInFront.ReplaceAllString(strings.TrimSpace(s), "")
	var out []string
	for _, part := range directorSep.Split(s, -1) {
		part = roleInFront.ReplaceAllString(strings.TrimSpace(part), "")
		part = cleanName(roleAfter.ReplaceAllString(part, ""))
		if part == "" {
			continue
		}
		if !LooksLikeName(part) {
			break
		}
		out = append(out, part)
	}
	return out
}

func labelWord(label string) string {
	l := strings.ToLower(strings.Join(strings.Fields(label), " "))
	switch {
	case strings.HasPrefix(l, "geschäftsf"), strings.HasPrefix(l, "vertret"), strings.HasPrefix(l, "managing"), strings.HasPrefix(l, "represented"), l == "ceo":
		return "managing director"
	case strings.HasPrefix(l, "inhaber"), l == "owner":
		return "owner"
	case l == "vorstand":
		return "board"
	}
	return "managing director"
}

// notStartups are words in a company name that mark a bank, a public body
// or an association rather than a young company.
var notStartups = regexp.MustCompile(`(?i)(?:sparkasse|bank|volksbank|stadt |stadtwerke|universität|university|hochschule|institut|verband|kammer|stiftung|foundation|ministerium|landes|kreis |gemeinde|verein|agentur für arbeit|wirtschaftsförderung|genossenschaft)`)

// Young says whether the imprint reads like a young company run by the
// people it names: a GmbH, UG, sole trader or partnership, with at most four
// managing directors. A stock company, an association, a bank or a public
// body is not, and nobody in its imprint counts as a founder.
func (im Imprint) Young() bool {
	if len(im.Directors) == 0 || len(im.Directors) > 4 || im.Label == "board" {
		return false
	}
	switch strings.ReplaceAll(im.LegalForm, " ", "") {
	case "AG", "SE", "e.V.", "gGmbH", "KG", "B.V.":
		return false
	}
	return !notStartups.MatchString(im.Company + " ")
}
