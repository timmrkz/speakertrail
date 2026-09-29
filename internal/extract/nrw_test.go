package extract

import "testing"

func TestInNRW(t *testing.T) {
	for pc, want := range map[string]bool{
		"50667": true, "40210": true, "44137": true, "48143": true, "52062": true, "53111": true, "33602": true,
		"32423": true, "57072": true, "59065": true, "34414": true, "37671": true, "49477": true, "53721": true,
		"53604": true, "48477": true,
		// Next door: Lower Saxony, Hesse, Rhineland-Palatinate.
		"49074": false, "31134": false, "34117": false, "37073": false, "53424": false, "53474": false,
		"53619": false, "57518": false, "48455": false, "48529": false, "54290": false, "80331": false,
		"": false, "5066": false, "abcde": false,
	} {
		if got := InNRW(pc); got != want {
			t.Errorf("InNRW(%q) = %v, want %v", pc, got, want)
		}
	}
}

// A directory lists businesses all over Germany. The postcode next to an
// entry says where it is, so entries outside NRW never need a lookup.
// All invented.
func TestDirectoryEntriesCarryTheirPostcode(t *testing.T) {
	page := `<html><body><nav><a href="/login">Login</a></nav><main><h1>Gyms</h1><ul>
<li><a href="https://beispiel-bjj.test/">Beispiel BJJ</a><br>Musterstraße 1, 50667 Köln</li>
<li><a href="https://muster-kampfsport.test/">Muster Kampfsport</a> <span>80331 München</span></li>
<li><a href="https://probe-dojo.test/">Probe Dojo</a></li>
</ul></main></body></html>`
	got := Portfolio(page, "https://gyms.test/list").Startups
	want := []Startup{
		{Name: "Beispiel BJJ", Website: "https://beispiel-bjj.test/", Postcode: "50667"},
		{Name: "Muster Kampfsport", Website: "https://muster-kampfsport.test/", Postcode: "80331"},
		{Name: "Probe Dojo", Website: "https://probe-dojo.test/"},
	}
	if len(got) != len(want) {
		t.Fatalf("entries %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Entries in one block together say nothing about each other's postcode.
func TestOneBlockOfEntriesHasNoPostcode(t *testing.T) {
	page := `<main><div><a href="https://a-gym.test/">A Gym</a> 50667 Köln <a href="https://b-gym.test/">B Gym</a> 80331 München
<a href="https://c-gym.test/">C Gym</a> 10115 Berlin <a href="https://d-gym.test/">D Gym</a> 20095 Hamburg</div></main>`
	for _, s := range Portfolio(page, "https://gyms.test/").Startups {
		if s.Postcode != "" {
			t.Errorf("%s got postcode %s from its neighbours", s.Name, s.Postcode)
		}
	}
}
