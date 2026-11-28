package search

import (
	"net/url"
	"slices"
	"strings"
	"time"
)

// The person a LinkedIn page Tim pasted belongs to, wherever he pasted
// it, is found by its links and by its name (Tim, 24 November 2026,
// #89). The engine holds the people, see engine.Whose.

// PageLinks are the names in the profile links of a page, as a profile's
// link has them after /in/, most often first. Chrome copies the page's
// HTML with its text, and the window hands the links in it. The page's
// own name stands in many of them, like /in/mara-beispiel/overlay/
// contact-info/ and /in/mara-beispiel/recent-activity/all/, other
// people's in one or two. A link outside LinkedIn has none.
func PageLinks(hrefs []string) []string {
	count := map[string]int{}
	for _, h := range hrefs[:min(len(hrefs), 5000)] {
		u, err := url.Parse(strings.TrimSpace(h))
		if err != nil || u.Host != "" && u.Host != "linkedin.com" && !strings.HasSuffix(u.Host, ".linkedin.com") {
			continue
		}
		rest, ok := strings.CutPrefix(u.Path, "/in/")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		if name = strings.ToLower(name); name != "" {
			count[name]++
		}
	}
	names := make([]string, 0, len(count))
	for n := range count {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if count[a] != count[b] {
			return count[b] - count[a]
		}
		return strings.Compare(a, b)
	})
	return names
}

// LinkNamed is where the name in link stands in names, from PageLinks,
// or -1 when it is not there.
func LinkNamed(link string, names []string) int {
	n := linkName(link)
	if n == "" {
		return -1
	}
	return slices.Index(names, n)
}

// PageName is the name on a page, read where LinkedIn puts it, right
// above how near Tim is to them, empty when the page has no such line.
func PageName(text string) string {
	p, _ := Pasted(text, "", time.Time{})
	return p.Name
}

// SameName says whether the name on a page is a person's, the name the
// app has for them or the name in their link, each also without
// initials, with ä written as a or ae, see spelled.
func SameName(page, name, link string) bool {
	on := []string{spelled(page), spelled(withoutInitials(page))}
	if on[0] == "" {
		return false
	}
	theirs := append([]string{spelled(name), spelled(withoutInitials(strings.ReplaceAll(name, "-", " ")))}, linkNames(link)...)
	for _, a := range on {
		for _, b := range theirs {
			if a != "" && a == b {
				return true
			}
		}
	}
	return false
}

// OwnLink is the link of the profile a page shows, from the links in its
// HTML: the one in the name on the page, or else the one it holds most
// often, written the way an added link is, https://www.linkedin.com/in/
// and the name in it. Empty when the HTML holds no profile link, as
// when only its text was pasted.
func OwnLink(hrefs []string, name string) string {
	names := PageLinks(hrefs)
	if len(names) == 0 {
		return ""
	}
	own := names[0]
	for _, n := range names {
		if SameName(name, "", "https://www.linkedin.com/in/"+n) {
			own = n
			break
		}
	}
	// The name as the link spells it, in its own capitals.
	for _, h := range hrefs[:min(len(hrefs), 5000)] {
		u, err := url.Parse(strings.TrimSpace(h))
		if err != nil {
			continue
		}
		if rest, ok := strings.CutPrefix(u.Path, "/in/"); ok {
			if n, _, _ := strings.Cut(rest, "/"); strings.ToLower(n) == own {
				return "https://www.linkedin.com/in/" + n
			}
		}
	}
	return ""
}

// FirstDegree says whether a page shows Tim connected with them: "· 1st"
// or "· 1." under the name, as LinkedIn writes it for a connection.
func FirstDegree(text string) bool {
	top, _ := sectioned(text)
	for _, l := range top {
		if degree.MatchString(l) {
			d := strings.TrimSpace(strings.TrimLeft(l, "·• "))
			return strings.HasPrefix(d, "1st") || strings.HasPrefix(d, "1.")
		}
	}
	return false
}
