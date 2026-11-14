package place

import (
	"regexp"
	"sort"
	"strings"
)

// NRWCities are the cities the engine recognises in places and queries.
var NRWCities = []string{
	"Köln", "Bonn", "Düsseldorf", "Essen", "Dortmund", "Duisburg", "Bochum", "Aachen", "Münster", "Wuppertal", "Bielefeld",
	"Gelsenkirchen", "Mönchengladbach", "Krefeld", "Oberhausen", "Hagen", "Hamm", "Mülheim an der Ruhr", "Leverkusen",
	"Solingen", "Herne", "Neuss", "Paderborn", "Bottrop", "Recklinghausen", "Remscheid", "Bergisch Gladbach", "Moers",
	"Siegen", "Gütersloh", "Witten", "Iserlohn", "Düren", "Ratingen", "Lünen", "Marl", "Velbert", "Minden", "Viersen",
	"Troisdorf", "Rheine", "Dorsten", "Castrop-Rauxel", "Arnsberg", "Detmold", "Lüdenscheid", "Bocholt", "Grevenbroich",
	"Unna", "Dinslaken", "Herford", "Kerpen", "Lippstadt", "Bergheim", "Dormagen", "Gladbeck", "Sankt Augustin", "Wesel",
	"Hürth", "Siegburg", "Frechen", "Brühl", "Hilden", "Langenfeld", "Monheim am Rhein", "Erkrath", "Meerbusch", "Kleve",
	"Euskirchen", "Königswinter", "Bad Honnef", "Wesseling", "Pulheim", "Leichlingen", "Kamp-Lintfort", "Emmerich",
}

// Other spellings that mean the same city.
var cityAliases = map[string]string{
	"cologne": "Köln", "koeln": "Köln", "koln": "Köln", "dusseldorf": "Düsseldorf", "duesseldorf": "Düsseldorf",
	"muenster": "Münster", "munster": "Münster", "monchengladbach": "Mönchengladbach", "moenchengladbach": "Mönchengladbach",
	"mulheim": "Mülheim an der Ruhr", "muelheim": "Mülheim an der Ruhr", "mülheim": "Mülheim an der Ruhr",
	"gutersloh": "Gütersloh", "guetersloh": "Gütersloh", "duren": "Düren", "dueren": "Düren", "hurth": "Hürth", "huerth": "Hürth",
	"ludenscheid": "Lüdenscheid", "luenen": "Lünen", "lunen": "Lünen", "aix-la-chapelle": "Aachen", "st. augustin": "Sankt Augustin",
}

var cityPatterns = func() []struct {
	re   *regexp.Regexp
	city string
} {
	names := map[string]string{}
	for _, c := range NRWCities {
		names[strings.ToLower(c)] = c
	}
	for alias, c := range cityAliases {
		names[alias] = c
	}
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	// Longest first, so "Mülheim an der Ruhr" wins over "Mülheim".
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	var out []struct {
		re   *regexp.Regexp
		city string
	}
	for _, k := range keys {
		out = append(out, struct {
			re   *regexp.Regexp
			city string
		}{regexp.MustCompile(`(?i)(?:^|[^\p{L}])` + regexp.QuoteMeta(k) + `(?:$|[^\p{L}])`), names[k]})
	}
	return out
}()

var postcodeCity = regexp.MustCompile(`\b\d{5}\s+([\p{L}][\p{L}.\- ]{1,40})`)

// CityOf finds the NRW city named in the given texts, first match wins.
// It returns the city after a postcode when no known city is named.
func CityOf(texts ...string) string {
	for _, t := range texts {
		if t == "" {
			continue
		}
		for _, p := range cityPatterns {
			if p.re.MatchString(t) {
				return p.city
			}
		}
	}
	for _, t := range texts {
		if m := postcodeCity.FindStringSubmatch(t); m != nil {
			return strings.TrimSpace(strings.Split(m[1], ",")[0])
		}
	}
	return ""
}
