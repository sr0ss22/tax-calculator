package estimate

import (
	"math"
	"strings"
	"testing"
)

func TestParseAvalaraCSV(t *testing.T) {
	// Standard Avalara export header + a couple rows, plus a percent-style row and
	// a ZIP+4 to exercise normalization and the percent->fraction heuristic.
	const sample = `State,ZipCode,TaxRegionName,EstimatedCombinedRate,StateRate,EstimatedCountyRate,EstimatedCityRate,EstimatedSpecialRate,RiskLevel
MD,21562,FROSTBURG,0.06,0.06,0,0,0,1
TX,78664,ROUND ROCK,0.0825,0.0625,0,0.01,0.01,1
NY,10001-1234,NEW YORK CITY,8.875%,0.04,0.04,0,0.00375,2
`
	out := map[string]zipRate{}
	parseAvalaraCSV(strings.NewReader(sample), out)

	cases := map[string]float64{"21562": 0.06, "78664": 0.0825, "10001": 0.08875}
	for zip, want := range cases {
		zr, ok := out[zip]
		if !ok {
			t.Fatalf("zip %s not loaded", zip)
		}
		if math.Abs(zr.combined-want) > 1e-9 {
			t.Errorf("zip %s combined = %v, want %v", zip, zr.combined, want)
		}
	}
	if zr := out["78664"]; zr.region != "ROUND ROCK" {
		t.Errorf("region = %q, want ROUND ROCK", zr.region)
	}
}

func TestParseRate(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"0.06", 0.06, true},
		{"6", 0.06, true},
		{"6%", 0.06, true},
		{"0.08250", 0.0825, true},
		{"", 0, false},
		{"n/a", 0, false},
	}
	for _, c := range cases {
		got, ok := parseRate(c.in)
		if ok != c.ok || (ok && math.Abs(got-c.want) > 1e-9) {
			t.Errorf("parseRate(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
