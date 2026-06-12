package estimate

// stateAverageCombinedRate is the per-state average combined (state + average
// local) sales tax rate, as a fraction. Source: Tax Foundation, "State and Local
// Sales Tax Rates, 2026" (Table 1, population-weighted average local rates).
// https://taxfoundation.org/data/all/state/sales-tax-rates/
//
// This is the offline US rate source: it needs no API and no per-ZIP dataset. It
// is a state-level AVERAGE, so it does not capture the exact local rate at a
// specific address; for an exact rate the caller passes a rate override (or a
// TaxJar token enables live ZIP lookup). Estimate only; SAP remains the system of
// record for actual tax. Refresh annually when the Tax Foundation updates Table 1.
var stateAverageCombinedRate = map[string]float64{
	"Alabama":              0.0946,
	"Alaska":               0.0182,
	"Arizona":              0.0852,
	"Arkansas":             0.0946,
	"California":           0.0899,
	"Colorado":             0.0789,
	"Connecticut":          0.0635,
	"Delaware":             0.0000,
	"District of Columbia": 0.0600,
	"Florida":              0.0698,
	"Georgia":              0.0749,
	"Hawaii":               0.0450,
	"Idaho":                0.0603,
	"Illinois":             0.0896,
	"Indiana":              0.0700,
	"Iowa":                 0.0694,
	"Kansas":               0.0869,
	"Kentucky":             0.0600,
	"Louisiana":            0.1011,
	"Maine":                0.0550,
	"Maryland":             0.0600,
	"Massachusetts":        0.0625,
	"Michigan":             0.0600,
	"Minnesota":            0.0814,
	"Mississippi":          0.0706,
	"Missouri":             0.0844,
	"Montana":              0.0000,
	"Nebraska":             0.0698,
	"Nevada":               0.0824,
	"New Hampshire":        0.0000,
	"New Jersey":           0.0660,
	"New Mexico":           0.0767,
	"New York":             0.0854,
	"North Carolina":       0.0700,
	"North Dakota":         0.0709,
	"Ohio":                 0.0729,
	"Oklahoma":             0.0906,
	"Oregon":               0.0000,
	"Pennsylvania":         0.0634,
	"Rhode Island":         0.0700,
	"South Carolina":       0.0749,
	"South Dakota":         0.0611,
	"Tennessee":            0.0961,
	"Texas":                0.0820,
	"Utah":                 0.0742,
	"Vermont":              0.0639,
	"Virginia":             0.0577,
	"Washington":           0.0951,
	"West Virginia":        0.0659,
	"Wisconsin":            0.0572,
	"Wyoming":              0.0556,
}

// stateAverageRate returns the average combined rate for a full state name. ok is
// false when the state is not in the table (an unresolved state value).
func stateAverageRate(stateName string) (float64, bool) {
	r, ok := stateAverageCombinedRate[stateName]
	return r, ok
}
