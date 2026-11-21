package search

import "testing"

// A profile's text keeps everything but the ways to reach the person:
// email addresses and phone numbers go, the dates of jobs stay.
func TestWithoutContact(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Schreib mir: mara@beispiel-keramik.de oder ruf an.", "Schreib mir: oder ruf an."},
		{"Tel. +49 221 1234567\nKeramik", "Tel.\nKeramik"},
		{"Mobil 0171-1234567, Studio 0221 / 123 45 67.", "Mobil , Studio ."},
		{"Inhaberin @ Beispiel Keramik", "Inhaberin @ Beispiel Keramik"},
		{"Keramikerin\n2019 - 2023 · 4 Jahre", "Keramikerin\n2019 - 2023 · 4 Jahre"},
		{"01.03.2020 - 31.12.2022", "01.03.2020 - 31.12.2022"},
		{"10/2019 - 03/2023", "10/2019 - 03/2023"},
		{"50667 Köln, 1.200 Follower", "50667 Köln, 1.200 Follower"},
	} {
		if got := WithoutContact(tc.in); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.in, got, tc.want)
		}
	}
}
