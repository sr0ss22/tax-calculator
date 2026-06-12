package estimate

import (
	"context"
	"math"
	"strings"
	"testing"
)

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func newEstimator(t *testing.T) *Estimator {
	t.Helper()
	e, err := New("") // no TaxJar; US relies on rate override
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return e
}

func TestEstimate_TexasTHD_Blended_WithOverride(t *testing.T) {
	e := newEstimator(t)
	got, err := e.Estimate(context.Background(), Request{
		Channel:      "THD",
		State:        "TX",
		Zip:          "78664",
		RateOverride: 0.0825,
		Lines: []Line{
			{Name: "Blinds", Category: "blinds", Amount: 1500},
			{Name: "Shutters", Category: "shutters", Amount: 2000},
		},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if got.Country != "US" || got.State != "Texas" {
		t.Errorf("country/state = %q/%q, want US/Texas", got.Country, got.State)
	}
	// TX THD: blinds taxable, shutters exempt.
	if !approxEq(got.TaxableBase, 1500) {
		t.Errorf("TaxableBase = %v, want 1500", got.TaxableBase)
	}
	if !approxEq(got.TotalTax, 1500*0.0825) {
		t.Errorf("TotalTax = %v, want %v", got.TotalTax, 1500*0.0825)
	}
	if !got.Blended {
		t.Errorf("Blended = false, want true for THD blinds+shutters in Texas")
	}
	if !got.RateOverridden {
		t.Errorf("RateOverridden = false, want true")
	}
}

func TestEstimate_FallsBackToStateAverage(t *testing.T) {
	e := newEstimator(t)
	// No override and no TaxJar token -> the static TX state average combined rate.
	got, err := e.Estimate(context.Background(), Request{
		State: "TX",
		Lines: []Line{{Name: "Blinds", Category: "blinds", Amount: 1000}},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if !approxEq(got.CombinedRate, 0.0820) {
		t.Errorf("CombinedRate = %v, want 0.0820 (TX state average)", got.CombinedRate)
	}
	if !approxEq(got.TotalTax, 82) {
		t.Errorf("TotalTax = %v, want 82 (1000 * 0.0820)", got.TotalTax)
	}
	if !got.RateEstimated {
		t.Errorf("RateEstimated = false, want true for a state-average estimate")
	}
	found := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "state average") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a state-average warning, got %v", got.Warnings)
	}
}

func TestEstimate_CanadaOntario(t *testing.T) {
	e := newEstimator(t)
	got, err := e.Estimate(context.Background(), Request{
		State: "Ontario",
		Lines: []Line{
			{Name: "Blinds", Category: "blinds", Amount: 1000},
			{Name: "Drapes", Category: "draperies", Amount: 1000},
		},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if got.Country != "Canada" || got.State != "Ontario" {
		t.Errorf("country/state = %q/%q, want Canada/Ontario", got.Country, got.State)
	}
	if !approxEq(got.TotalTax, 2000*0.13) {
		t.Errorf("TotalTax = %v, want %v (Ontario 13%%)", got.TotalTax, 2000*0.13)
	}
	if !approxEq(got.CombinedRate, 0.13) {
		t.Errorf("effective rate = %v, want 0.13", got.CombinedRate)
	}
}

func TestEstimate_CanadaBC_DraperyException(t *testing.T) {
	e := newEstimator(t)
	got, err := e.Estimate(context.Background(), Request{
		State: "BC",
		Lines: []Line{
			{Name: "Blinds", Category: "blinds", Amount: 1000},    // 5%
			{Name: "Drapes", Category: "draperies", Amount: 1000}, // 12%
		},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	// 1000*0.05 + 1000*0.12 = 170; effective 170/2000 = 0.085.
	if !approxEq(got.TotalTax, 170) {
		t.Errorf("TotalTax = %v, want 170 (BC drapery 12%%, blinds 5%%)", got.TotalTax)
	}
	if !approxEq(got.CombinedRate, 0.085) {
		t.Errorf("effective rate = %v, want 0.085 (blended), not a flat 5%%", got.CombinedRate)
	}
}

func lineByName(res Result, name string) (LineResult, bool) {
	for _, l := range res.Lines {
		if l.Name == name {
			return l, true
		}
	}
	return LineResult{}, false
}

func TestEstimate_PerCategoryInstall_Illinois(t *testing.T) {
	// Illinois (partner): drapery product AND drapery install are taxable, but
	// blinds product and blinds install are both exempt. A single lumped install
	// fee could not express this; separate per-category install lines can.
	e := newEstimator(t)
	got, err := e.Estimate(context.Background(), Request{
		Channel:      "partners",
		State:        "Illinois",
		RateOverride: 0.07,
		Lines: []Line{
			{Name: "Drapery", Category: "draperies", Amount: 1000, Kind: "product"},
			{Name: "Blinds", Category: "blinds", Amount: 1000, Kind: "product"},
			{Name: "Install Drapery", Category: "draperies", Amount: 500, Kind: "install"},
			{Name: "Install Blinds", Category: "blinds", Amount: 500, Kind: "install"},
		},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	// Taxable: drapery product 1000 + drapery install 500 = 1500; tax 1500*0.07 = 105.
	if !approxEq(got.TaxableBase, 1500) {
		t.Errorf("TaxableBase = %v, want 1500 (drapery product + drapery install)", got.TaxableBase)
	}
	if !approxEq(got.TotalTax, 105) {
		t.Errorf("TotalTax = %v, want 105", got.TotalTax)
	}
	checks := []struct {
		name        string
		wantTaxable bool
	}{
		{"Drapery", true},
		{"Blinds", false},
		{"Install Drapery", true},
		{"Install Blinds", false},
	}
	for _, c := range checks {
		l, ok := lineByName(got, c.name)
		if !ok {
			t.Fatalf("line %q missing from result", c.name)
		}
		if l.Taxable != c.wantTaxable {
			t.Errorf("line %q taxable = %v, want %v", c.name, l.Taxable, c.wantTaxable)
		}
	}
}

func TestEstimate_UnknownKind_Errors(t *testing.T) {
	e := newEstimator(t)
	_, err := e.Estimate(context.Background(), Request{
		State: "TX",
		Lines: []Line{{Name: "X", Category: "blinds", Amount: 1, Kind: "warranty"}},
	})
	if err == nil {
		t.Errorf("Estimate() error = nil, want an error for an unknown kind")
	}
}

func TestEstimate_Errors(t *testing.T) {
	e := newEstimator(t)
	tests := []struct {
		name string
		req  Request
	}{
		{"no state", Request{Lines: []Line{{Name: "B", Category: "blinds", Amount: 1}}}},
		{"no lines", Request{State: "TX"}},
		{"empty name", Request{State: "TX", Lines: []Line{{Name: "", Category: "blinds", Amount: 1}}}},
		{"negative amount", Request{State: "TX", Lines: []Line{{Name: "B", Category: "blinds", Amount: -1}}}},
		{"unknown category", Request{State: "TX", Lines: []Line{{Name: "B", Category: "rugs", Amount: 1}}}},
		{"negative override", Request{State: "TX", RateOverride: -0.01, Lines: []Line{{Name: "B", Category: "blinds", Amount: 1}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.Estimate(context.Background(), tt.req); err == nil {
				t.Errorf("Estimate() error = nil, want an error for %q", tt.name)
			}
		})
	}
}
