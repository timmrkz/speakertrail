package extract

import (
	"regexp"
	"strconv"
	"strings"
)

// InNRW says whether a German postcode lies in North Rhine-Westphalia.
// Most two-digit areas lie wholly in NRW. Where one crosses the border,
// the postcodes on the other side are listed: Lower Saxony around
// Nordhorn and Bad Bentheim, Rhineland-Palatinate along the Ahr, at Linz
// and in the Westerwald, Hesse around Kassel. The NRW parts of areas that
// mostly lie elsewhere, Warburg, Höxter and Ibbenbüren, are listed too.
func InNRW(postcode string) bool {
	if len(postcode) != 5 {
		return false
	}
	n, err := strconv.Atoi(postcode)
	if err != nil {
		return false
	}
	switch n / 1000 {
	case 32, 33, 40, 41, 42, 44, 45, 46, 47, 50, 51, 52, 58, 59:
		return true
	case 48:
		switch n {
		case 48455, 48465, 48480, 48488, 48499:
			return false
		}
		return n < 48527 || n > 48531
	case 53:
		return (n < 53424 || n > 53579) && n != 53619
	case 57:
		return n < 57500
	case 34:
		return n >= 34414 && n <= 34439
	case 37:
		return n == 37671 || n == 37688 || n == 37696
	case 49:
		return n >= 49477 && n <= 49549
	}
	return false
}

// nrwState names North Rhine-Westphalia in a place line.
var nrwState = regexp.MustCompile(`(?i)^(?:north rhine-westphalia|nordrhein-westfalen|nrw)$`)

// ruhrRegion is the Ruhr as a region of its own, like LinkedIn's "Ruhr
// Region".
var ruhrRegion = regexp.MustCompile(`(?i)(?:^|[^\p{L}])ruhr(?:gebiet)?(?:$|[^\p{L}])`)

// PlaceInNRW reads a place as a profile gives it, like "Cologne, North
// Rhine-Westphalia, Germany", "Ruhr Region" or "Greater Dusseldorf Area",
// and says whether it lies in NRW, with the city in German when it names
// one. A place that names another state or country, or only Germany,
// does not count.
func PlaceInNRW(place string) (city string, ok bool) {
	var parts []string
	for _, p := range strings.Split(place, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	if nrwState.MatchString(parts[0]) {
		return "", true
	}
	if len(parts) > 1 && !nrwState.MatchString(parts[1]) && !isGermany(parts[1]) {
		return "", false
	}
	if len(parts) > 1 && nrwState.MatchString(parts[1]) {
		if c := CityOf(parts[0]); c != "" {
			return c, true
		}
		return parts[0], true
	}
	if c := CityOf(parts[0]); c != "" {
		return c, true
	}
	return "", ruhrRegion.MatchString(parts[0])
}

func isGermany(s string) bool {
	s = strings.ToLower(s)
	return s == "germany" || s == "deutschland"
}
