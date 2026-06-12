package taxestimate

import "math"

// round2 rounds a dollar amount to whole cents. Per-line tax is rounded here so
// the displayed line taxes sum exactly to the total (matching ERP per-line
// rounding); otherwise sum-of-rounded-parts can differ from the raw total by a cent.
func round2(x float64) float64 { return math.Round(x*100) / 100 }

// blendStates is the set of states where, for THD, blinds and shutters are taxed
// differently, so a job mixing both needs one weighted blended rate per the
// Blended Tax SOP. Ported verbatim from the RLN prototype BLEND_STATES.
var blendStates = map[string]bool{
	"Arizona":        true,
	"Colorado":       true,
	"Connecticut":    true,
	"Florida":        true,
	"Georgia":        true,
	"Illinois":       true,
	"Indiana":        true,
	"Kansas":         true,
	"Maryland":       true,
	"Michigan":       true,
	"Minnesota":      true,
	"New York":       true,
	"Oklahoma":       true,
	"Pennsylvania":   true,
	"Rhode Island":   true,
	"South Carolina": true,
	"South Dakota":   true,
	"Texas":          true,
	"Utah":           true,
	"West Virginia":  true,
	"Wisconsin":      true,
}

// lineTypesForOrderType returns the product and labor line types for an order
// type. A Job pairs an Installed Package with Additional Labor Services; a
// Service Order pairs a Product with Installation Labor.
func lineTypesForOrderType(ot OrderType) (product, labor LineType, ok bool) {
	switch ot {
	case OrderTypeJob:
		return LineTypeInstalledPackage, LineTypeAdditionalLabor, true
	case OrderTypeServiceOrder:
		return LineTypeProduct, LineTypeInstallationLabor, true
	default:
		// Unknown order type: do not coerce to Job. The caller skips the
		// divergence check so an upstream mapping defect is not masked.
		return "", "", false
	}
}

// LaborDivergesFromProduct reports whether, for the given channel, state,
// category, and order type, the labor line type is taxed differently from the
// product line type. Callers that default every line to the product line type
// use this to warn that labor-only lines may be mis-estimated (for example Iowa
// taxes installation labor while exempting the product).
func (m *Matrix) LaborDivergesFromProduct(channel Channel, state string, category Category, orderType OrderType) bool {
	productLT, laborLT, ok := lineTypesForOrderType(orderType)
	if !ok {
		// Unknown order type: cannot say labor diverges, and coercing to Job
		// behavior would emit a misleading warning. Treat as non-divergent.
		return false
	}
	productTaxable, productFound := m.Taxable(MatrixKey{Channel: channel, State: state, Category: category, OrderType: orderType, LineType: productLT})
	laborTaxable, laborFound := m.Taxable(MatrixKey{Channel: channel, State: state, Category: category, OrderType: orderType, LineType: laborLT})
	return productFound && laborFound && productTaxable != laborTaxable
}

// TaxLineInput is one quote line reduced to what the matrix and tax math need.
// Amount is the line retail amount in dollars. Categorizing the line (from its
// product classification), choosing the order and line type, and reading the
// amount are the integration layer's job; the Calculator is pure.
type TaxLineInput struct {
	Category  Category
	OrderType OrderType
	LineType  LineType
	Amount    float64
}

// LineTaxResult is the per-line outcome.
type LineTaxResult struct {
	Input TaxLineInput
	// Found is false when the matrix has no entry for this line; such a line is
	// flagged for review (shown N/A) and excluded from the taxable base.
	Found bool
	// Taxable is true only when Found and the matrix flag is true.
	Taxable bool
	// AppliedRate is the rate used for this line (the order rate when taxable, else 0).
	AppliedRate float64
	// Tax is Amount*AppliedRate when taxable, else 0.
	Tax float64
	// LaborMayDiverge is true when labor is taxed differently from product for
	// this line's channel/state/category/order type. Because the line type is
	// defaulted to product, a labor-only line of this kind may be mis-estimated.
	LaborMayDiverge bool
}

// OrderTaxResult is the whole-order outcome.
type OrderTaxResult struct {
	Lines []LineTaxResult
	// Rate is the combined rate applied to taxable lines.
	Rate float64
	// TaxableBase is the sum of taxable line amounts.
	TaxableBase float64
	// Retail is the sum of all line amounts (taxable or not).
	Retail float64
	// TotalTax is the sum of per-line tax.
	TotalTax float64
	// OrderTotal is Retail + TotalTax.
	OrderTotal float64
	// HasUnmapped is true when any line was not found in the matrix (flagged for review).
	HasUnmapped bool
	// Blended is true for the THD mixed blinds+shutters divergent-state case.
	Blended bool
	// BlendedRate is TotalTax/Retail, the single weighted rate to enter on the
	// TAX and TAX-Install lines per the Blended Tax SOP. Set only when Blended.
	BlendedRate float64
	// LaborMayDiverge is true when any line is in a channel/state/category where
	// labor is taxed differently from product. Since line types default to
	// product, labor-only lines may be mis-estimated; callers surface a warning.
	LaborMayDiverge bool
}

// Calculator applies the taxability matrix and computes per-line and order tax.
type Calculator struct {
	matrix *Matrix
}

// NewCalculator builds a Calculator over a loaded matrix.
func NewCalculator(matrix *Matrix) *Calculator {
	return &Calculator{matrix: matrix}
}

// Compute applies the matrix and rate to the lines and returns the per-line and
// order-level tax. rate is the combined rate for the order ZIP (from RateService).
func (c *Calculator) Compute(channel Channel, state string, rate float64, lines []TaxLineInput) OrderTaxResult {
	result := OrderTaxResult{
		Rate:  rate,
		Lines: make([]LineTaxResult, 0, len(lines)),
	}
	var hasBlinds, hasShutters bool

	for _, line := range lines {
		taxable, found := c.matrix.Taxable(MatrixKey{
			Channel:   channel,
			State:     state,
			Category:  line.Category,
			OrderType: line.OrderType,
			LineType:  line.LineType,
		})

		lineResult := LineTaxResult{Input: line, Found: found, Taxable: found && taxable}
		if lineResult.Taxable {
			lineResult.AppliedRate = rate
			lineResult.Tax = round2(line.Amount * rate)
			result.TaxableBase += line.Amount
		}
		if c.matrix.LaborDivergesFromProduct(channel, state, line.Category, line.OrderType) {
			lineResult.LaborMayDiverge = true
			result.LaborMayDiverge = true
		}
		result.TotalTax += lineResult.Tax
		result.Retail += line.Amount
		if !found {
			result.HasUnmapped = true
		}

		switch line.Category {
		case CategoryBlinds:
			hasBlinds = true
		case CategoryShutters:
			hasShutters = true
		}
		result.Lines = append(result.Lines, lineResult)
	}

	result.TotalTax = round2(result.TotalTax)
	result.OrderTotal = round2(result.Retail + result.TotalTax)

	if channel == ChannelTHD && hasBlinds && hasShutters && blendStates[state] {
		result.Blended = true
		if result.Retail > 0 {
			result.BlendedRate = result.TotalTax / result.Retail
		}
	}
	return result
}
