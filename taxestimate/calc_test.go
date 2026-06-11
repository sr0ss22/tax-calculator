package taxestimate

import (
	"math"
	"testing"
)

func newTestCalculator(t *testing.T) *Calculator {
	t.Helper()
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	return NewCalculator(m)
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func blindsJob(amount float64) TaxLineInput {
	return TaxLineInput{Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage, Amount: amount}
}

func shuttersJob(amount float64) TaxLineInput {
	return TaxLineInput{Category: CategoryShutters, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage, Amount: amount}
}

func TestCompute_StandardOrder_TaxableAndNonTaxableLines(t *testing.T) {
	c := newTestCalculator(t)
	// Texas THD: Blinds installed taxable, Shutters installed exempt.
	lines := []TaxLineInput{blindsJob(1000), shuttersJob(2000)}
	got := c.Compute(ChannelTHD, "Texas", 0.0825, lines)

	if got.Retail != 3000 {
		t.Errorf("Retail = %v, want 3000", got.Retail)
	}
	if got.TaxableBase != 1000 {
		t.Errorf("TaxableBase = %v, want 1000 (only blinds taxable)", got.TaxableBase)
	}
	if !approx(got.TotalTax, 82.5) {
		t.Errorf("TotalTax = %v, want 82.5", got.TotalTax)
	}
	if !approx(got.OrderTotal, 3082.5) {
		t.Errorf("OrderTotal = %v, want 3082.5", got.OrderTotal)
	}
	if got.HasUnmapped {
		t.Errorf("HasUnmapped = true, want false")
	}
	// Per-line assertions
	if !got.Lines[0].Taxable || !approx(got.Lines[0].Tax, 82.5) {
		t.Errorf("blinds line = %+v, want taxable with tax 82.5", got.Lines[0])
	}
	if got.Lines[1].Taxable || got.Lines[1].Tax != 0 {
		t.Errorf("shutters line = %+v, want non-taxable with 0 tax", got.Lines[1])
	}
}

func TestCompute_NoTaxState_AllExempt(t *testing.T) {
	c := newTestCalculator(t)
	// Oregon: nothing taxable. Rate is irrelevant.
	lines := []TaxLineInput{blindsJob(1000), shuttersJob(500)}
	got := c.Compute(ChannelTHD, "Oregon", 0.0, lines)

	if got.TaxableBase != 0 || got.TotalTax != 0 {
		t.Errorf("Oregon should yield no tax, got base=%v tax=%v", got.TaxableBase, got.TotalTax)
	}
	if got.OrderTotal != 1500 {
		t.Errorf("OrderTotal = %v, want 1500 (retail only)", got.OrderTotal)
	}
	if got.Blended {
		t.Errorf("Oregon is not a blend state, want Blended=false")
	}
}

func TestCompute_THDBlendedCase(t *testing.T) {
	c := newTestCalculator(t)
	// The prototype's blended example: THD, Texas, blinds 1500 + shutters 2500.
	// Texas blinds taxable, shutters exempt. Blended = tax/retail.
	lines := []TaxLineInput{blindsJob(1500), shuttersJob(2500)}
	got := c.Compute(ChannelTHD, "Texas", 0.0825, lines)

	if !got.Blended {
		t.Fatalf("Blended = false, want true for THD blinds+shutters in Texas")
	}
	wantTax := 1500 * 0.0825 // only blinds taxable
	if !approx(got.TotalTax, wantTax) {
		t.Errorf("TotalTax = %v, want %v", got.TotalTax, wantTax)
	}
	wantBlended := wantTax / 4000.0 // tax / total retail
	if !approx(got.BlendedRate, wantBlended) {
		t.Errorf("BlendedRate = %v, want %v", got.BlendedRate, wantBlended)
	}
}

func TestCompute_NotBlended_WhenNotMixed(t *testing.T) {
	c := newTestCalculator(t)
	// Texas THD but blinds only: not a blended case.
	got := c.Compute(ChannelTHD, "Texas", 0.0825, []TaxLineInput{blindsJob(1000)})
	if got.Blended {
		t.Errorf("Blended = true, want false with no shutters")
	}
}

func TestCompute_NotBlended_NonBlendState(t *testing.T) {
	c := newTestCalculator(t)
	// California is not in the blend-state list, even with blinds + shutters.
	got := c.Compute(ChannelTHD, "California", 0.0725, []TaxLineInput{blindsJob(1000), shuttersJob(1000)})
	if got.Blended {
		t.Errorf("Blended = true, want false for a non-blend state")
	}
}

func TestCompute_NotBlended_NonTHDChannel(t *testing.T) {
	c := newTestCalculator(t)
	got := c.Compute(ChannelPartners, "Texas", 0.0825, []TaxLineInput{blindsJob(1000), shuttersJob(1000)})
	if got.Blended {
		t.Errorf("Blended = true, want false for a non-THD channel")
	}
}

func TestCompute_UnmappedLine_FlaggedAndExcluded(t *testing.T) {
	c := newTestCalculator(t)
	// A line whose key is not in the matrix (bogus state) is flagged and excluded.
	lines := []TaxLineInput{
		blindsJob(1000),
		{Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage, Amount: 500}, // Atlantis below
	}
	lines[1].Amount = 500
	got := c.Compute(ChannelTHD, "Atlantis", 0.0825, lines)

	if !got.HasUnmapped {
		t.Errorf("HasUnmapped = false, want true for an unknown state")
	}
	if got.TaxableBase != 0 {
		t.Errorf("TaxableBase = %v, want 0 (no matrix entry)", got.TaxableBase)
	}
	if got.Retail != 1500 {
		t.Errorf("Retail = %v, want 1500 (retail still counts unmapped lines)", got.Retail)
	}
	for i, l := range got.Lines {
		if l.Found {
			t.Errorf("line %d Found = true, want false for unknown state", i)
		}
	}
}

func TestCompute_EmptyLines(t *testing.T) {
	c := newTestCalculator(t)
	got := c.Compute(ChannelTHD, "Texas", 0.0825, nil)
	if got.Retail != 0 || got.TotalTax != 0 || got.OrderTotal != 0 {
		t.Errorf("empty order should be all zero, got %+v", got)
	}
	if got.Blended {
		t.Errorf("empty order should not be blended")
	}
	if len(got.Lines) != 0 {
		t.Errorf("Lines = %d, want 0", len(got.Lines))
	}
}

func TestMatrix_LaborDivergesFromProduct(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	tests := []struct {
		name      string
		state     string
		orderType OrderType
		want      bool
	}{
		// Iowa THD: product exempt, labor taxable -> diverges (both Job and Service Order).
		{name: "Iowa Job diverges", state: "Iowa", orderType: OrderTypeJob, want: true},
		{name: "Iowa Service Order diverges", state: "Iowa", orderType: OrderTypeServiceOrder, want: true},
		// Texas THD Blinds: product taxable and labor taxable the same -> no divergence.
		{name: "Texas Job does not diverge", state: "Texas", orderType: OrderTypeJob, want: false},
		// Oregon: all exempt -> no divergence.
		{name: "Oregon Job does not diverge", state: "Oregon", orderType: OrderTypeJob, want: false},
		// Unknown state -> not found -> no divergence (cannot compare).
		{name: "unknown state does not diverge", state: "Atlantis", orderType: OrderTypeJob, want: false},
		// Unknown order type is not coerced to Job; it cannot diverge so the warning is suppressed.
		{name: "unknown order type does not diverge", state: "Iowa", orderType: OrderType("Lease"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.LaborDivergesFromProduct(ChannelTHD, tt.state, CategoryBlinds, tt.orderType)
			if got != tt.want {
				t.Errorf("LaborDivergesFromProduct(THD, %q, Blinds, %q) = %v, want %v", tt.state, tt.orderType, got, tt.want)
			}
		})
	}
}

func TestCompute_BlendedZeroRetail_NoDivideByZero(t *testing.T) {
	c := newTestCalculator(t)
	// Mixed blinds+shutters in a blend state but zero amounts: must not divide by zero.
	got := c.Compute(ChannelTHD, "Texas", 0.0825, []TaxLineInput{blindsJob(0), shuttersJob(0)})
	if !got.Blended {
		t.Errorf("Blended = false, want true (still the blended scenario)")
	}
	if got.BlendedRate != 0 {
		t.Errorf("BlendedRate = %v, want 0 when retail is 0", got.BlendedRate)
	}
}
