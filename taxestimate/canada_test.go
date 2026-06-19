package taxestimate

import (
	"math"
	"testing"
)

func approxEqCA(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestLoadCanada_ThirteenProvinces(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	if got := len(c.Provinces()); got != 13 {
		t.Errorf("Provinces() = %d, want 13 (the provinces/territories on the Canada chart)", got)
	}
}

func TestDefaultCanada_Memoized(t *testing.T) {
	c1, err := DefaultCanada()
	if err != nil {
		t.Fatalf("DefaultCanada() error = %v", err)
	}
	c2, err := DefaultCanada()
	if err != nil {
		t.Fatalf("DefaultCanada() second call error = %v", err)
	}
	if c1 != c2 {
		t.Errorf("DefaultCanada() returned distinct instances; want the same memoized pointer")
	}
}

func TestCanadaRates_ResolveProvince(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	tests := []struct {
		raw      string
		wantName string
		wantOK   bool
	}{
		{"Ontario", "Ontario", true},
		{"ON", "Ontario", true},
		{"bc", "British Columbia", true},
		{"British Columbia", "British Columbia", true},
		{"Texas", "", false},
		{"TX", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		gotName, gotOK := c.ResolveProvince(tt.raw)
		if gotOK != tt.wantOK || gotName != tt.wantName {
			t.Errorf("ResolveProvince(%q) = (%q, %v), want (%q, %v)", tt.raw, gotName, gotOK, tt.wantName, tt.wantOK)
		}
	}
}

func TestCanadaRates_Rate(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	tests := []struct {
		name     string
		province string
		category Category
		lineType LineType
		wantRate float64
		wantOK   bool
	}{
		{"Alberta GST only", "Alberta", CategoryBlinds, LineTypeProduct, 0.05, true},
		{"Ontario HST", "Ontario", CategoryBlinds, LineTypeProduct, 0.13, true},
		{"Quebec GST+QST", "Quebec", CategoryShutters, LineTypeProduct, 0.14975, true},
		{"Nova Scotia HST 14", "Nova Scotia", CategoryBlinds, LineTypeProduct, 0.14, true},
		{"Saskatchewan GST+PST 11", "Saskatchewan", CategoryBlinds, LineTypeProduct, 0.11, true},
		// British Columbia: PST only on the draperies/workroom PRODUCT line.
		{"BC draperies product = 12", "British Columbia", CategoryDraperies, LineTypeProduct, 0.12, true},
		{"BC blinds product = 5", "British Columbia", CategoryBlinds, LineTypeProduct, 0.05, true},
		{"BC draperies labor = 5", "British Columbia", CategoryDraperies, LineTypeAdditionalLabor, 0.05, true},
		// Manitoba: GST + PST = 12 on every line.
		{"MB blinds product = 12", "Manitoba", CategoryBlinds, LineTypeProduct, 0.12, true},
		{"MB install labor = 12", "Manitoba", CategoryBlinds, LineTypeAdditionalLabor, 0.12, true},
		// Unknown province does not resolve.
		{"unknown province", "Texas", CategoryBlinds, LineTypeProduct, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, _, ok := c.Rate(tt.province, tt.category, tt.lineType)
			if ok != tt.wantOK {
				t.Fatalf("Rate(%q,...) found = %v, want %v", tt.province, ok, tt.wantOK)
			}
			if ok && !approxEqCA(rate, tt.wantRate) {
				t.Errorf("Rate(%q, %q, %q) = %v, want %v", tt.province, tt.category, tt.lineType, rate, tt.wantRate)
			}
		})
	}
}

func TestCanadaRates_Compute_Ontario(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	lines := []TaxLineInput{
		{Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeProduct, Amount: 1000},
		{Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeAdditionalLabor, Amount: 200},
	}
	got := c.Compute("Ontario", lines)
	// Ontario taxes everything at 13%.
	if !approxEqCA(got.Retail, 1200) {
		t.Errorf("Retail = %v, want 1200", got.Retail)
	}
	if !approxEqCA(got.TaxableBase, 1200) {
		t.Errorf("TaxableBase = %v, want 1200 (Canada taxes every line)", got.TaxableBase)
	}
	if !approxEqCA(got.TotalTax, 156) { // 1200 * 0.13
		t.Errorf("TotalTax = %v, want 156", got.TotalTax)
	}
	if got.HasUnmapped {
		t.Errorf("HasUnmapped = true, want false (every Ontario line resolves)")
	}
	for _, l := range got.Lines {
		if !l.Taxable {
			t.Errorf("line %+v should be taxable in Canada", l.Input)
		}
	}
}

func TestCanadaRates_Compute_BCDraperyException(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	lines := []TaxLineInput{
		{Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeProduct, Amount: 1000},           // 5%
		{Category: CategoryDraperies, OrderType: OrderTypeJob, LineType: LineTypeProduct, Amount: 1000},        // 12%
		{Category: CategoryDraperies, OrderType: OrderTypeJob, LineType: LineTypeAdditionalLabor, Amount: 500}, // 5%
	}
	got := c.Compute("British Columbia", lines)
	// 1000*0.05 + 1000*0.12 + 500*0.05 = 50 + 120 + 25 = 195.
	if !approxEqCA(got.TotalTax, 195) {
		t.Errorf("BC TotalTax = %v, want 195 (drapery product taxed at 12%%, blinds + labor at 5%%)", got.TotalTax)
	}
}

func TestLoadCanada_MalformedComponentFailsFast(t *testing.T) {
	cases := map[string]string{
		"wrong arity": `{"canada":{"provinces":[{"name":"X","combined":5,"components":[["GST"]]}]}}`,
		"non-string":  `{"canada":{"provinces":[{"name":"X","combined":5,"components":[[5,5]]}]}}`,
		"non-number":  `{"canada":{"provinces":[{"name":"X","combined":5,"components":[["GST","five"]]}]}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadCanada([]byte(raw)); err == nil {
				t.Errorf("loadCanada with a %s component should fail fast, got nil error", name)
			}
		})
	}
}

func TestCanadaRates_Rate_ReturnsCopy(t *testing.T) {
	c, err := LoadCanada()
	if err != nil {
		t.Fatalf("LoadCanada() error = %v", err)
	}
	// Ontario takes the default branch (returns the stored components).
	_, comps, _ := c.Rate("Ontario", CategoryBlinds, LineTypeProduct)
	if len(comps) == 0 {
		t.Fatalf("expected components for Ontario")
	}
	comps[0].Percent = 999 // mutate the returned slice
	_, comps2, _ := c.Rate("Ontario", CategoryBlinds, LineTypeProduct)
	if comps2[0].Percent == 999 {
		t.Errorf("Rate returned a shared slice; caller mutation leaked into stored state")
	}
}
