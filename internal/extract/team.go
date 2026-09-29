package extract

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	teamText = regexp.MustCompile(`(?i)^(?:team|unser team|das team|our team|the team|über uns|ueber uns|uber uns|about|about us|wir|wer wir sind|who we are|people|founders|gründer|gründerteam)$`)
	teamPath = regexp.MustCompile(`(?i)/(?:[a-z]{2}/)?(?:team|unser-team|our-team|about|about-us|ueber-uns|uber-uns|über-uns|wir|founders|gruender)(?:\.html?|\.php|/)?$`)
)

// TeamLink finds the page of a website that introduces its team, like
// "Team" or "Über uns". It returns "" when there is none.
func TeamLink(body, base string) string {
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
		if teamText.MatchString(strings.TrimSpace(l.Text)) {
			return l.URL
		}
		if byPath == "" && teamPath.MatchString(u.Path) {
			byPath = l.URL
		}
	}
	return byPath
}

// personalProfile matches a profile of one person on a platform, not a
// company page or a post: linkedin.com/in/…, xing.com/profile/…, x.com/….
var personalProfile = regexp.MustCompile(`(?i)^https?://(?:[a-z]+\.)?(?:linkedin\.com/in/|xing\.com/profile/|(?:x|twitter)\.com/|instagram\.com/|github\.com/)([^/?#]+)/?$`)

// ProfileLinks lists the links of a page that lead to a person's own
// profile on a platform. The engine never loads them, it only keeps them.
func ProfileLinks(body, base string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range Links(body, base) {
		if personalProfile.MatchString(l.URL) && !seen[l.URL] {
			seen[l.URL] = true
			out = append(out, l.URL)
		}
	}
	return out
}

// ProfilesOf keeps the profile links that belong to this person: the
// address carries their last name and their first name or its initial, like
// linkedin.com/in/lena-musterfrau-4b2a1 or x.com/lmusterfrau. A link without
// the name, which a team page may put next to anyone, is never guessed.
func ProfilesOf(links []string, name string) []string {
	// "Jürgen" is written "Juergen" in an address as often as "Jurgen".
	umlauts := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue")
	var forms [][2]string
	for _, n := range []string{name, umlauts.Replace(name)} {
		words := strings.Fields(NormaliseName(n))
		if len(words) < 2 {
			return nil
		}
		forms = append(forms, [2]string{words[0], words[len(words)-1]})
	}
	var out []string
	for _, l := range links {
		m := personalProfile.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		slug, err := url.PathUnescape(m[1])
		if err != nil {
			continue
		}
		slug = strings.ReplaceAll(NormaliseName(strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(slug)), " ", "")
		for _, f := range forms {
			first, last := f[0], f[1]
			i := strings.Index(slug, last)
			if i >= 0 && (strings.Contains(slug, first) || (i > 0 && slug[i-1] == first[0])) {
				out = append(out, l)
				break
			}
		}
	}
	return out
}
