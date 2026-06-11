package taxestimate

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Canada taxes every window-covering line (product, installation, warranty,
// consultation) in every province. There are no Yes/No taxability flags like the
// US charts have, only rates, and the rates are static from the Canada window tax
// chart (Costco / Decorview / DirectBuy, updated 2025-04-02). So Canada needs no
// taxability matrix, no address-level lookup, and no external rate provider (no
// TaxJar): the province alone determines the rate.
//
// Two province-level exceptions are the only nuance:
//   - British Columbia charges PST (7%) ONLY on the Draperies and Workroom product
//     line (12% combined). Blinds, shutters, drapery hardware, all installation,
//     and consultation fees are GST-only (5%).
//   - Manitoba charges the Design Consultation Fee at GST only (5%); every other
//     line is GST + PST (12%).

// RateComponent is one part of a combined rate (GST, HST, PST, or QST). Percent is
// expressed in whole points, so 5 means 5%.
type RateComponent struct {
	Name    string
	Percent float64
}

// canadaProvince is one province row from the chart: its combined rate and the
// components that make it up. (The province name is the map key in CanadaRates,
// so it is not duplicated here.)
type canadaProvince struct {
	Components []RateComponent
	Combined   float64 // whole points, e.g. 13 for 13%
}

// CanadaRates is the static, province-keyed Canada rate table loaded from the seed.
type CanadaRates struct {
	byName map[string]canadaProvince
	order  []string
}

// canadaProvinceByCode resolves the two-letter province/territory codes to the
// full names used in the seed, so a quote location carrying "ON" or "BC" resolves.
var canadaProvinceByCode = map[string]string{
	"AB": "Alberta", "BC": "British Columbia", "MB": "Manitoba",
	"NB": "New Brunswick", "NL": "Newfoundland & Labrador", "NT": "Northwest Territories",
	"NS": "Nova Scotia", "NU": "Nunavut", "ON": "Ontario", "PE": "Prince Edward Island",
	"QC": "Quebec", "SK": "Saskatchewan", "YT": "Yukon Territory",
}

// --- seed parsing (the "canada" block of tax_data.json) ---

type canadaSeed struct {
	// Only the fields the rate logic reads are parsed; the seed's other canada keys
	// (all_taxable, note, channel, products, line_types, exceptions, source) are
	// intentionally ignored.
	Provinces []struct {
		Name       string          `json:"name"`
		Components [][]interface{} `json:"components"`
		Combined   float64         `json:"combined"`
	} `json:"provinces"`
}

type taxDataCanada struct {
	Canada *canadaSeed `json:"canada"`
}

var (
	defaultCanada     *CanadaRates
	defaultCanadaErr  error
	defaultCanadaOnce sync.Once
)

// DefaultCanada returns a process-wide memoized CanadaRates parsed from the same
// embedded seed as the US matrix. Request-path callers should use this.
func DefaultCanada() (*CanadaRates, error) {
	defaultCanadaOnce.Do(func() { defaultCanada, defaultCanadaErr = loadCanada(taxDataJSON) })
	return defaultCanada, defaultCanadaErr
}

// LoadCanada parses a fresh CanadaRates from the embedded seed (for tests/callers
// that want an independent instance).
func LoadCanada() (*CanadaRates, error) { return loadCanada(taxDataJSON) }

func loadCanada(raw []byte) (*CanadaRates, error) {
	var d taxDataCanada
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("taxestimate: parse canada seed: %w", err)
	}
	if d.Canada == nil || len(d.Canada.Provinces) == 0 {
		return nil, fmt.Errorf("taxestimate: canada seed missing or empty")
	}
	cr := &CanadaRates{byName: make(map[string]canadaProvince, len(d.Canada.Provinces))}
	for _, p := range d.Canada.Provinces {
		comps := make([]RateComponent, 0, len(p.Components))
		for _, c := range p.Components {
			if len(c) != 2 {
				return nil, fmt.Errorf("taxestimate: canada province %q has a malformed rate component %v (want [name, percent])", p.Name, c)
			}
			name, okName := c[0].(string)
			pct, okPct := c[1].(float64)
			if !okName || !okPct {
				return nil, fmt.Errorf("taxestimate: canada province %q has a non-(string, number) rate component %v", p.Name, c)
			}
			comps = append(comps, RateComponent{Name: name, Percent: pct})
		}
		cr.byName[p.Name] = canadaProvince{Components: comps, Combined: p.Combined}
		cr.order = append(cr.order, p.Name)
	}
	return cr, nil
}

// ResolveProvince maps a location value (full name or two-letter code) to the
// canonical province name in the table. Returns ok=false when it is not a
// Canadian province, which is how the estimator decides a quote is not Canadian.
func (c *CanadaRates) ResolveProvince(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if _, ok := c.byName[s]; ok {
		return s, true
	}
	if full, ok := canadaProvinceByCode[strings.ToUpper(s)]; ok {
		if _, ok2 := c.byName[full]; ok2 {
			return full, true
		}
	}
	return "", false
}

// IsProvince reports whether raw resolves to a Canadian province.
func (c *CanadaRates) IsProvince(raw string) bool {
	_, ok := c.ResolveProvince(raw)
	return ok
}

// Provinces lists the province names in chart order.
func (c *CanadaRates) Provinces() []string {
	return append([]string(nil), c.order...)
}

func isProductLine(lt LineType) bool {
	return lt == LineTypeInstalledPackage || lt == LineTypeProduct
}

// Rate returns the combined rate (as a fraction, 0.13 = 13%) and its components
// for a line in a province. Canada taxes every line, so found is true whenever the
// province resolves. The BC and MB exceptions are applied here.
func (c *CanadaRates) Rate(province string, category Category, lineType LineType) (rate float64, components []RateComponent, found bool) {
	name, ok := c.ResolveProvince(province)
	if !ok {
		return 0, nil, false
	}
	p := c.byName[name]
	gst := RateComponent{Name: "GST", Percent: 5}
	pst := RateComponent{Name: "PST", Percent: 7}

	switch name {
	case "British Columbia":
		// PST applies only to the Draperies and Workroom product line; everything
		// else (blinds, shutters, all installation labor, consult fee) is GST-only.
		if category == CategoryDraperies && isProductLine(lineType) {
			return 0.12, []RateComponent{gst, pst}, true
		}
		return 0.05, []RateComponent{gst}, true
	case "Manitoba":
		if lineType == LineTypeConsultationFee {
			return 0.05, []RateComponent{gst}, true
		}
		return 0.12, []RateComponent{gst, pst}, true
	default:
		// Defensive copy: p.Components is stored in CanadaRates.byName, so returning
		// it directly would let a caller mutate shared state. (The BC/MB branches
		// above already return freshly-built slices.)
		return p.Combined / 100.0, append([]RateComponent(nil), p.Components...), true
	}
}

// Compute applies the static province rate to each line and returns the per-line
// and order-level tax in the same shape as the US Calculator. Every line is
// taxable when the province resolves; there is no blended-rate case and no
// labor-divergence in Canada. Lines whose province does not resolve are flagged
// not-found and excluded (this should not happen because the estimator only routes
// resolved-province quotes here).
func (c *CanadaRates) Compute(province string, lines []TaxLineInput) OrderTaxResult {
	result := OrderTaxResult{Lines: make([]LineTaxResult, 0, len(lines))}
	for _, line := range lines {
		rate, _, found := c.Rate(province, line.Category, line.LineType)
		lr := LineTaxResult{Input: line, Found: found, Taxable: found}
		if found {
			lr.AppliedRate = rate
			lr.Tax = line.Amount * rate
			result.TaxableBase += line.Amount
		} else {
			result.HasUnmapped = true
		}
		result.TotalTax += lr.Tax
		result.Retail += line.Amount
		result.Lines = append(result.Lines, lr)
	}
	result.OrderTotal = result.Retail + result.TotalTax
	return result
}
