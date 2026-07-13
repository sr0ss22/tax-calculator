package estimate

import (
	"encoding/csv"
	"strings"
)

// azCityRate is one Arizona city/jurisdiction combined rate from HD's
// authoritative "AZ Sales tax by City" chart. A city can appear multiple times
// (different jurisdictions and rates), so lookups return every matching entry.
type azCityRate struct {
	City         string
	Rate         float64 // combined rate as a fraction (0.087 = 8.7%)
	Jurisdiction string
}

// azCityRates is the loaded AZ city rate table (HD authoritative), built once at
// startup from estimate/ratedata/az-city-rates.csv.
//
// This is a REFERENCE and VALIDATION dataset, not a rate-path override. The
// channel charts say Arizona rates come from this AZ city chart; the live rate
// path (TaxJar, or the offline Avalara ZIP table) already returns AZ rates by
// ZIP, so the calculator keeps using the ZIP-based lookup and this table is here
// to cross-check that path and to serve as an offline AZ reference. It is not
// wired into the estimate because the calculator keys rates by ZIP, not by city
// (a city here can span several jurisdictions with different rates).
var azCityRates = loadAZCityRates()

func loadAZCityRates() map[string][]azCityRate {
	out := map[string][]azCityRate{}
	f, err := rateDataFS.Open("ratedata/az-city-rates.csv")
	if err != nil {
		return out
	}
	defer f.Close()

	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return out
	}
	col := func(names ...string) int {
		for i, h := range header {
			hn := normHeader(h)
			for _, n := range names {
				if hn == n {
					return i
				}
			}
		}
		return -1
	}
	ci := col("city")
	ri := col("rate", "combinedrate")
	ji := col("jurisdiction", "region")
	if ci < 0 || ri < 0 {
		return out
	}
	for {
		rec, err := cr.Read()
		if err != nil {
			break
		}
		if ci >= len(rec) || ri >= len(rec) {
			continue
		}
		rate, ok := parseRate(rec[ri])
		if !ok {
			continue
		}
		city := strings.TrimSpace(rec[ci])
		if city == "" {
			continue
		}
		juris := ""
		if ji >= 0 && ji < len(rec) {
			juris = strings.TrimSpace(rec[ji])
		}
		key := strings.ToLower(city)
		out[key] = append(out[key], azCityRate{City: city, Rate: rate, Jurisdiction: juris})
	}
	return out
}

// azCityRatesFor returns every rate entry for an Arizona city (case-insensitive).
// ok is false when the city is not in the table. More than one entry means the
// city spans jurisdictions with different rates; the caller picks or shows a range.
func azCityRatesFor(city string) ([]azCityRate, bool) {
	e, ok := azCityRates[strings.ToLower(strings.TrimSpace(city))]
	return e, ok
}

// azCityRateCount returns the total number of AZ city rate rows loaded, for
// validation that the reference table embedded and parsed.
func azCityRateCount() int {
	n := 0
	for _, v := range azCityRates {
		n += len(v)
	}
	return n
}
