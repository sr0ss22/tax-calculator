package estimate

import (
	"embed"
	"encoding/csv"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"github.com/sr0ss22/tax-calculator/taxestimate"
)

// rateDataFS holds the Avalara monthly ZIP rate tables. Drop the free per-state
// CSV downloads (https://www.avalara.com/taxrates/en/download-tax-tables.html)
// into estimate/ratedata/ and they are embedded at build time. The directory is
// embedded whole so the build works even before any CSV is added; non-CSV files
// (the README) are ignored at load time.
//
//go:embed ratedata
var rateDataFS embed.FS

// zipRate is one ZIP's combined rate plus its Avalara tax-region label.
type zipRate struct {
	combined float64 // combined state+local rate as a fraction (0.0825 = 8.25%)
	region   string  // TaxRegionName, for display/provenance
}

// zipRates is the loaded ZIP -> rate table, built once at startup. Empty when no
// Avalara CSV has been added, in which case lookups miss and the estimator falls
// back to the per-state average.
var zipRates = loadZipRates()

func loadZipRates() map[string]zipRate {
	out := map[string]zipRate{}
	entries, err := fs.ReadDir(rateDataFS, "ratedata")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".csv") {
			continue
		}
		f, err := rateDataFS.Open("ratedata/" + e.Name())
		if err != nil {
			continue
		}
		parseAvalaraCSV(f, out)
		_ = f.Close()
	}
	return out
}

// parseAvalaraCSV reads an Avalara "sales tax rates by ZIP" CSV into out. It maps
// columns by header name (tolerant of spacing/case), so the standard export
// (State, ZipCode, TaxRegionName, EstimatedCombinedRate, StateRate,
// EstimatedCountyRate, EstimatedCityRate, EstimatedSpecialRate, RiskLevel) loads
// without configuration. The first row seen for a ZIP wins.
func parseAvalaraCSV(r io.Reader, out map[string]zipRate) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate ragged rows
	header, err := cr.Read()
	if err != nil {
		return
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
	zi := col("zipcode", "zip", "postalcode")
	ri := col("estimatedcombinedrate", "combinedrate", "taxrate", "totalsalestaxrate")
	ni := col("taxregionname", "region", "county", "taxregion")
	if zi < 0 || ri < 0 {
		return
	}
	for {
		rec, err := cr.Read()
		if err != nil {
			break
		}
		if zi >= len(rec) || ri >= len(rec) {
			continue
		}
		z, ok := taxestimate.NormalizeZip(rec[zi])
		if !ok {
			continue
		}
		if _, exists := out[z]; exists {
			continue // keep first occurrence
		}
		rate, ok := parseRate(rec[ri])
		if !ok {
			continue
		}
		region := ""
		if ni >= 0 && ni < len(rec) {
			region = strings.TrimSpace(rec[ni])
		}
		out[z] = zipRate{combined: rate, region: region}
	}
}

// normHeader lowercases a header and strips everything but letters and digits so
// "Estimated Combined Rate", "EstimatedCombinedRate", and "estimated_combined_rate"
// all compare equal.
func normHeader(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseRate parses a rate cell as a fraction. Avalara exports fractions (0.06),
// but a value > 1 is treated as a percent (6 -> 0.06) and a trailing % is allowed.
func parseRate(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	if v > 1 {
		v /= 100
	}
	return v, true
}

// zipRateFor returns the combined rate for a raw ZIP from the Avalara table.
func zipRateFor(rawZip string) (zipRate, bool) {
	z, ok := taxestimate.NormalizeZip(rawZip)
	if !ok {
		return zipRate{}, false
	}
	zr, ok := zipRates[z]
	return zr, ok
}
