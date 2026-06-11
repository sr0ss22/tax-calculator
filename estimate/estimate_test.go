package estimate

import (
	"context"
	"math"
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

func TestEstimate_NoRateSource_Warns(t *testing.T) {
	e := newEstimator(t)
	got, err := e.Estimate(context.Background(), Request{
		State: "TX",
		Lines: []Line{{Name: "Blinds", Category: "blinds", Amount: 1000}},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if got.TotalTax != 0 {
		t.Errorf("TotalTax = %v, want 0 with no rate source", got.TotalTax)
	}
	if len(got.Warnings) == 0 {
		t.Errorf("expected a no-rate-source warning")
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

func TestEstimate_InstallFeeSplit(t *testing.T) {
	e := newEstimator(t)
	// Ontario, all 13%: 1000 blinds + 1000 drapes + 600 install -> 2600 * 0.13.
	got, err := e.Estimate(context.Background(), Request{
		State:      "Ontario",
		InstallFee: 600,
		Lines: []Line{
			{Name: "Blinds", Category: "blinds", Amount: 1000},
			{Name: "Drapes", Category: "draperies", Amount: 1000},
		},
	})
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if !approxEq(got.Retail, 2600) {
		t.Errorf("Retail = %v, want 2600 (install split adds to retail)", got.Retail)
	}
	if !approxEq(got.TotalTax, 2600*0.13) {
		t.Errorf("TotalTax = %v, want %v", got.TotalTax, 2600*0.13)
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
