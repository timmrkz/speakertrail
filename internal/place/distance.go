package place

import "math"

// centres are where the cities of NRWCities lie, by their centres as
// GeoNames gives them, from its list of places with more than 1000 people,
// read on 21 November 2026. GeoNames is shared under CC BY 4.0.
var centres = map[string][2]float64{
	"Köln":                {50.9333, 6.9500},
	"Bonn":                {50.7344, 7.0955},
	"Düsseldorf":          {51.2232, 6.7793},
	"Essen":               {51.4566, 7.0123},
	"Dortmund":            {51.5149, 7.4660},
	"Duisburg":            {51.4325, 6.7652},
	"Bochum":              {51.4817, 7.2165},
	"Aachen":              {50.7766, 6.0834},
	"Münster":             {51.9624, 7.6257},
	"Wuppertal":           {51.2563, 7.1482},
	"Bielefeld":           {52.0333, 8.5333},
	"Gelsenkirchen":       {51.5051, 7.0965},
	"Mönchengladbach":     {51.1854, 6.4417},
	"Krefeld":             {51.3364, 6.5538},
	"Oberhausen":          {51.4781, 6.8625},
	"Hagen":               {51.3608, 7.4717},
	"Hamm":                {51.6803, 7.8209},
	"Mülheim an der Ruhr": {51.4322, 6.8797},
	"Leverkusen":          {51.0303, 6.9843},
	"Solingen":            {51.1734, 7.0845},
	"Herne":               {51.5388, 7.2257},
	"Neuss":               {51.1981, 6.6850},
	"Paderborn":           {51.7191, 8.7544},
	"Bottrop":             {51.5239, 6.9285},
	"Recklinghausen":      {51.6138, 7.1974},
	"Remscheid":           {51.1798, 7.1925},
	"Bergisch Gladbach":   {50.9856, 7.1330},
	"Moers":               {51.4534, 6.6326},
	"Siegen":              {50.8748, 8.0243},
	"Gütersloh":           {51.9069, 8.3785},
	"Witten":              {51.4436, 7.3526},
	"Iserlohn":            {51.3755, 7.7028},
	"Düren":               {50.8043, 6.4930},
	"Ratingen":            {51.2972, 6.8493},
	"Lünen":               {51.6163, 7.5287},
	"Marl":                {51.6567, 7.0904},
	"Velbert":             {51.3354, 7.0435},
	"Minden":              {52.2895, 8.9146},
	"Viersen":             {51.2544, 6.3944},
	"Troisdorf":           {50.8090, 7.1497},
	"Rheine":              {52.2822, 7.4413},
	"Dorsten":             {51.6617, 6.9651},
	"Castrop-Rauxel":      {51.5566, 7.3116},
	"Arnsberg":            {51.3833, 8.0833},
	"Detmold":             {51.9385, 8.8732},
	"Lüdenscheid":         {51.2198, 7.6273},
	"Bocholt":             {51.8388, 6.6153},
	"Grevenbroich":        {51.0910, 6.5827},
	"Unna":                {51.5380, 7.6897},
	"Dinslaken":           {51.5623, 6.7434},
	"Herford":             {52.1146, 8.6734},
	"Kerpen":              {50.8699, 6.6969},
	"Lippstadt":           {51.6737, 8.3448},
	"Bergheim":            {50.9557, 6.6399},
	"Dormagen":            {51.0968, 6.8317},
	"Gladbeck":            {51.5708, 6.9859},
	"Sankt Augustin":      {50.7754, 7.1970},
	"Wesel":               {51.6669, 6.6204},
	"Hürth":               {50.8708, 6.8676},
	"Siegburg":            {50.8002, 7.2077},
	"Frechen":             {50.9149, 6.8118},
	"Brühl":               {50.8293, 6.9050},
	"Hilden":              {51.1682, 6.9309},
	"Langenfeld":          {51.1082, 6.9483},
	"Monheim am Rhein":    {51.0916, 6.8922},
	"Erkrath":             {51.2223, 6.9083},
	"Meerbusch":           {51.2527, 6.6881},
	"Kleve":               {51.7883, 6.1387},
	"Euskirchen":          {50.6606, 6.7872},
	"Königswinter":        {50.6773, 7.1925},
	"Bad Honnef":          {50.6434, 7.2278},
	"Wesseling":           {50.8271, 6.9747},
	"Pulheim":             {50.9997, 6.8063},
	"Leichlingen":         {51.1063, 7.0187},
	"Kamp-Lintfort":       {51.5047, 6.5459},
	"Emmerich":            {51.8393, 6.2479},
}

// earthKm is the Earth's mean radius.
const earthKm = 6371.0

// Km is the distance in a straight line between two cities NRWCities
// names, in kilometres, and false when either is not one of them. It is
// not the way by road, which no request to a map is made for.
func Km(a, b string) (float64, bool) {
	p, ok := centres[a]
	q, ok2 := centres[b]
	if !ok || !ok2 {
		return 0, false
	}
	rad := math.Pi / 180
	lat1, lat2 := p[0]*rad, q[0]*rad
	dlat, dlon := lat2-lat1, (q[1]-p[1])*rad
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * earthKm * math.Asin(math.Sqrt(h)), true
}
