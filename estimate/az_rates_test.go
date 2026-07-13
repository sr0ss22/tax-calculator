package estimate

import "testing"

func TestAZCityRates_Loaded(t *testing.T) {
	// The HD AZ city chart has ~280 rows across ~260 cities.
	if n := azCityRateCount(); n < 250 {
		t.Errorf("azCityRateCount() = %d, want >= 250 (HD AZ city rate table)", n)
	}

	// Tucson spans several jurisdictions, so it has multiple rate entries.
	tucson, ok := azCityRatesFor("Tucson")
	if !ok || len(tucson) < 2 {
		t.Errorf("azCityRatesFor(Tucson) = %d entries (ok=%v), want multiple", len(tucson), ok)
	}

	// Lookup is case-insensitive.
	if _, ok := azCityRatesFor("goodyear"); !ok {
		t.Errorf("azCityRatesFor(goodyear) not found; case-insensitive lookup should match Goodyear")
	}

	// Rates are sane fractions.
	for _, e := range tucson {
		if e.Rate <= 0 || e.Rate > 0.15 {
			t.Errorf("Tucson rate %v out of expected range for %q", e.Rate, e.Jurisdiction)
		}
	}

	// Unknown city is not found.
	if _, ok := azCityRatesFor("Atlantis"); ok {
		t.Errorf("azCityRatesFor(Atlantis) should not be found")
	}
}
