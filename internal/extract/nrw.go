package extract

import "strconv"

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
