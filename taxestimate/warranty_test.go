package taxestimate

import "testing"

func TestLoadWarranty(t *testing.T) {
	w, err := LoadWarranty()
	if err != nil {
		t.Fatalf("LoadWarranty() error = %v", err)
	}
	// 52 states x 3 categories x 4 line types = 624 rows.
	if got := w.Len(); got != 624 {
		t.Errorf("Warranty.Len() = %d, want 624 (update only if the warranty seed change was intentional)", got)
	}
	if !w.ArizonaUsesCityTable() {
		t.Errorf("ArizonaUsesCityTable() = false, want true (chart says Arizona uses the AZ tax chart)")
	}
}

func TestWarranty_Taxable(t *testing.T) {
	w, err := LoadWarranty()
	if err != nil {
		t.Fatalf("LoadWarranty() error = %v", err)
	}
	ip := func(state string, cat Category) (bool, bool) {
		return w.Taxable(state, cat, OrderTypeJob, LineTypeInstalledPackage)
	}
	// Warranty shutters are taxable in Alabama and California, where the SERVICE-CALL
	// chart exempts them. This divergence is the whole reason warranty is its own grid.
	if tax, found := ip("Alabama", CategoryShutters); !found || !tax {
		t.Errorf("warranty AL shutters = (%v,%v), want (true,true)", tax, found)
	}
	if tax, found := ip("California", CategoryShutters); !found || !tax {
		t.Errorf("warranty CA shutters = (%v,%v), want (true,true)", tax, found)
	}
	// California warranty blinds installation labor (Job / Additional Labor) is NOT taxable.
	if tax, found := w.Taxable("California", CategoryBlinds, OrderTypeJob, LineTypeAdditionalLabor); !found || tax {
		t.Errorf("warranty CA blinds install = (%v,%v), want (false,true)", tax, found)
	}
	// Delaware (no sales tax) is exempt.
	if tax, found := ip("Delaware", CategoryBlinds); !found || tax {
		t.Errorf("warranty DE blinds = (%v,%v), want (false,true)", tax, found)
	}
	// Unknown state is not found.
	if _, found := ip("Atlantis", CategoryBlinds); found {
		t.Errorf("warranty Atlantis should not be found")
	}
}

func TestWarranty_RateOverride(t *testing.T) {
	w, err := LoadWarranty()
	if err != nil {
		t.Fatalf("LoadWarranty() error = %v", err)
	}
	cases := map[string]float64{
		"Illinois":      0.0625,
		"Missouri":      0.04225,
		"New Mexico":    0.05125,
		"Michigan":      0.06,
		"Minnesota":     0.06875,
		"New York City": 0.04375,
	}
	for state, want := range cases {
		got, ok := w.RateOverride(state)
		if !ok || got != want {
			t.Errorf("RateOverride(%q) = (%v,%v), want (%v,true)", state, got, ok, want)
		}
	}
	// A state with no override falls through to the combined rate.
	if _, ok := w.RateOverride("Texas"); ok {
		t.Errorf("RateOverride(Texas) ok = true, want false (no flat override)")
	}
}

func TestWarranty_DesignConsultationFee(t *testing.T) {
	w, err := LoadWarranty()
	if err != nil {
		t.Fatalf("LoadWarranty() error = %v", err)
	}
	if tax, found := w.DesignConsultationFeeTaxable("New York"); !found || !tax {
		t.Errorf("DCF New York = (%v,%v), want (true,true)", tax, found)
	}
	if tax, found := w.DesignConsultationFeeTaxable("Alabama"); !found || tax {
		t.Errorf("DCF Alabama = (%v,%v), want (false,true)", tax, found)
	}
}
