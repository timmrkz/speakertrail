package store

import "time"

// berlin is the time zone a month of budget is counted in.
var berlin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return loc
}()

func month(at time.Time) string { return at.In(berlin).Format("2006-01") }

// Use is how much of a provider's budget this month is spent.
type Use struct {
	Provider string `json:"provider"`
	Used     int    `json:"used"`
	Budget   int    `json:"budget"`
	// Set is true when the provider has an API key.
	Set bool `json:"set"`
}

// Used says how many calls a provider made in the month of at.
func (d *Data) Used(provider string, at time.Time) int {
	return d.Calls[provider][month(at)]
}

// Reserve counts a call to a provider before it is made, and reports false
// when this month's budget, the setting "<provider>_monthly_searches", is
// spent. Every call counts, a failed one too, because it may be billed.
func (d *Data) Reserve(provider string, at time.Time) bool {
	if d.Used(provider, at) >= d.Int(provider+"_monthly_searches") {
		return false
	}
	if d.Calls[provider] == nil {
		d.Calls[provider] = map[string]int{}
	}
	d.Calls[provider][month(at)]++
	return true
}
