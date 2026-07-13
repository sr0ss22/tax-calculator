package taxestimate

import (
	"encoding/json"
	"fmt"
)

// warrantyRow is one row of the channel-agnostic warranty taxability grid.
type warrantyRow struct {
	State     string `json:"state"`
	Product   string `json:"product"`
	OrderType string `json:"order_type"`
	LineType  string `json:"line_type"`
	Taxable   bool   `json:"taxable"`
}

// warrantyData is the on-disk shape of the "warranty" seed block.
type warrantyData struct {
	Note                  string             `json:"note"`
	ArizonaUsesCityTable  bool               `json:"arizona_uses_city_table"`
	RateOverrides         map[string]float64 `json:"rate_overrides"`
	DesignConsultationFee map[string]bool    `json:"design_consultation_fee"`
	Matrix                []warrantyRow      `json:"matrix"`
}

// warrantyKey identifies a warranty taxability decision. Unlike the main matrix
// it carries no channel: the warranty chart is channel-agnostic (one grid for
// Decorview / DirectBuy / Macy's / Sam's Club / JCP / THD).
type warrantyKey struct {
	State     string
	Category  Category
	OrderType OrderType
	LineType  LineType
}

// Warranty is the loaded warranty chart. Warranty fees are a flat $149-$249 fee
// taxed off THIS chart, not the new-job or service-call matrix, and where the
// chart lists a flat state-only rate that rate replaces the combined ZIP rate.
type Warranty struct {
	index         map[warrantyKey]bool
	rateOverrides map[string]float64
	dcf           map[string]bool
	azCityTable   bool
}

// LoadWarranty parses the "warranty" block embedded in the seed.
func LoadWarranty() (*Warranty, error) {
	var wrap struct {
		Warranty *warrantyData `json:"warranty"`
	}
	if err := json.Unmarshal(taxDataJSON, &wrap); err != nil {
		return nil, fmt.Errorf("taxestimate: parse warranty seed: %w", err)
	}
	if wrap.Warranty == nil || len(wrap.Warranty.Matrix) == 0 {
		return nil, fmt.Errorf("taxestimate: warranty block missing or empty")
	}
	w := &Warranty{
		index:         make(map[warrantyKey]bool, len(wrap.Warranty.Matrix)),
		rateOverrides: wrap.Warranty.RateOverrides,
		dcf:           wrap.Warranty.DesignConsultationFee,
		azCityTable:   wrap.Warranty.ArizonaUsesCityTable,
	}
	for i, r := range wrap.Warranty.Matrix {
		category := Category(r.Product)
		if !knownCategories[category] {
			return nil, fmt.Errorf("taxestimate: warranty row %d has unknown product %q", i, r.Product)
		}
		orderType := OrderType(r.OrderType)
		if !knownOrderTypes[orderType] {
			return nil, fmt.Errorf("taxestimate: warranty row %d has unknown order_type %q", i, r.OrderType)
		}
		lineType := LineType(r.LineType)
		if !knownLineTypes[lineType] {
			return nil, fmt.Errorf("taxestimate: warranty row %d has unknown line_type %q", i, r.LineType)
		}
		key := warrantyKey{State: r.State, Category: category, OrderType: orderType, LineType: lineType}
		if existing, dup := w.index[key]; dup && existing != r.Taxable {
			return nil, fmt.Errorf("taxestimate: warranty row %d duplicates key %+v with a conflicting taxable flag", i, key)
		}
		w.index[key] = r.Taxable
	}
	return w, nil
}

// Taxable reports whether a warranty line is taxable. found is false when the
// (state, category, order/line type) is absent from the warranty grid; callers
// flag such a line and exclude it from the taxable base.
func (w *Warranty) Taxable(state string, category Category, orderType OrderType, lineType LineType) (taxable, found bool) {
	t, ok := w.index[warrantyKey{State: state, Category: category, OrderType: orderType, LineType: lineType}]
	return t, ok
}

// RateOverride returns the flat state-only warranty rate for a state (or "New
// York City"), when the chart lists one. When ok is false the caller uses the
// normal combined rate.
func (w *Warranty) RateOverride(state string) (rate float64, ok bool) {
	r, ok := w.rateOverrides[state]
	return r, ok
}

// DesignConsultationFeeTaxable reports the Decorview-only Design Consultation Fee
// taxability for a state. found is false when the state is absent.
func (w *Warranty) DesignConsultationFeeTaxable(state string) (taxable, found bool) {
	t, ok := w.dcf[state]
	return t, ok
}

// ArizonaUsesCityTable reports whether Arizona warranty rates come from the AZ
// city rate table rather than a flat override or the combined rate.
func (w *Warranty) ArizonaUsesCityTable() bool { return w.azCityTable }

// Len returns the number of distinct warranty rows indexed.
func (w *Warranty) Len() int { return len(w.index) }
