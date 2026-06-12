// Package estimate is the standalone orchestration for the quote tax calculator.
// It turns an entered quote (lines, location, fees) into an estimated order tax
// using the pure taxestimate engine. It has no proto coupling and no dependency
// on any Brite service: the US rate comes from TaxJar (optional) or a manual
// override, and Canada uses the static province chart. Estimate only; SAP remains
// the system of record for actual tax.
package estimate

import (
	"context"
	"fmt"
	"strings"

	"github.com/sr0ss22/tax-calculator/taxestimate"
)

// Line is one entered line: a display name, a taxability category (blinds,
// shutters, or draperies), a dollar amount, and a kind. Kind is "product"
// (default) or "install"; installation labor is taxed by per-category rules that
// differ from product and from each other by state, so install blinds, install
// shutters, and install draperies are separate lines.
type Line struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Kind     string  `json:"kind"`
}

// Request is an entered quote to estimate.
type Request struct {
	Channel      string  `json:"channel"`      // "THD" (default) or "partners"
	State        string  `json:"state"`        // US state code/name, or Canadian province name/code
	Zip          string  `json:"zip"`          // US ZIP (for TaxJar); ignored for Canada
	RateOverride float64 `json:"rateOverride"` // optional US combined rate fraction (0.0825 = 8.25%)
	MeasureFee   float64 `json:"measureFee"`   // optional measure / design consultation fee
	Lines        []Line  `json:"lines"`
}

// LineResult is the per-line outcome for display.
type LineResult struct {
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Kind        string  `json:"kind"` // "product", "install", or "consult"
	LineType    string  `json:"lineType"`
	Amount      float64 `json:"amount"`
	Found       bool    `json:"found"`
	Taxable     bool    `json:"taxable"`
	AppliedRate float64 `json:"appliedRate"`
	Tax         float64 `json:"tax"`
	Warning     string  `json:"warning,omitempty"`
}

// Result is the whole-quote estimate.
type Result struct {
	Country        string       `json:"country"` // "US" or "Canada"
	State          string       `json:"state"`
	Zip            string       `json:"zip,omitempty"`
	CombinedRate   float64      `json:"combinedRate"`
	RateEstimated  bool         `json:"rateEstimated"`
	RateOverridden bool         `json:"rateOverridden"`
	// RateBoundaryRisk is true when the ZIP-table rate includes a city-level tax,
	// so a ZIP that straddles the city limit may be over/under-stated — verify.
	RateBoundaryRisk bool `json:"rateBoundaryRisk"`
	// RateCityRate is the city-tax component of a ZIP-table rate (fraction), so the
	// UI can tell the user how much to deduct if the address is outside city limits.
	RateCityRate float64 `json:"rateCityRate,omitempty"`
	TaxableBase    float64      `json:"taxableBase"`
	Retail         float64      `json:"retail"`
	TotalTax       float64      `json:"totalTax"`
	OrderTotal     float64      `json:"orderTotal"`
	Blended        bool         `json:"blended"`
	BlendedRate    float64      `json:"blendedRate"`
	Lines          []LineResult `json:"lines"`
	Warnings       []string     `json:"warnings"`
}

// rateLookup is the optional US rate provider (TaxJar). Canada never uses it.
type rateLookup interface {
	LookupRate(ctx context.Context, zip string) taxestimate.RateResult
}

// Estimator holds the loaded engine. Build once and reuse across requests.
type Estimator struct {
	calc   *taxestimate.Calculator
	canada *taxestimate.CanadaRates
	rates  rateLookup // nil when no TaxJar token is configured
}

// New loads the embedded matrix and Canada chart. When taxJarToken is non-empty,
// US rates are looked up live from TaxJar; otherwise US quotes require a manual
// rate override (Canada always works offline).
func New(taxJarToken string) (*Estimator, error) {
	matrix, err := taxestimate.LoadMatrix()
	if err != nil {
		return nil, fmt.Errorf("load matrix: %w", err)
	}
	canada, err := taxestimate.LoadCanada()
	if err != nil {
		return nil, fmt.Errorf("load canada: %w", err)
	}
	e := &Estimator{calc: taxestimate.NewCalculator(matrix), canada: canada}
	if taxJarToken != "" {
		e.rates = taxestimate.NewRateService(taxestimate.NewTaxJarProvider(taxJarToken, "", nil), nil)
	}
	return e, nil
}

// categoryFromString maps a label to a taxability category; ok=false if unknown.
func categoryFromString(s string) (taxestimate.Category, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "blinds", "blind":
		return taxestimate.CategoryBlinds, true
	case "shutters", "shutter":
		return taxestimate.CategoryShutters, true
	case "draperies", "drapery", "drapes", "drape":
		return taxestimate.CategoryDraperies, true
	default:
		return "", false
	}
}

// channelFor maps a label to a channel; THD is the default.
func channelFor(s string) taxestimate.Channel {
	if strings.EqualFold(strings.TrimSpace(s), "partners") || s == "2" {
		return taxestimate.ChannelPartners
	}
	return taxestimate.ChannelTHD
}

// builtLine pairs a calculator input with its display name.
type builtLine struct {
	input taxestimate.TaxLineInput
	name  string
	warn  string
	kind  string // "product", "install", or "consult" (for display grouping)
}

// buildLines turns the request into calculator inputs. productLineType is the line
// type for product lines (Installed Package for US, Product for Canada). A line is
// a product, an install, or a consultation fee:
//   - product -> the category's product taxability
//   - install -> the category's Additional Labor Services taxability (install
//     blinds, install shutters, and install draperies are taxed separately)
//   - consult -> the Design Consultation Fee line (no category)
//
// The legacy MeasureFee field is still honored as a consultation line.
func buildLines(req Request, productLineType taxestimate.LineType) ([]builtLine, error) {
	out := make([]builtLine, 0, len(req.Lines)+1)

	for i, l := range req.Lines {
		name := strings.TrimSpace(l.Name)
		if name == "" {
			return nil, fmt.Errorf("line %d: name is required", i+1)
		}
		if l.Amount < 0 {
			return nil, fmt.Errorf("line %d (%s): amount must be non-negative", i+1, name)
		}

		var input taxestimate.TaxLineInput
		kind := "product"
		switch strings.ToLower(strings.TrimSpace(l.Kind)) {
		case "consult", "consultation", "fee":
			// Consultation fee has its own category and line type; the category field
			// is not used.
			kind = "consult"
			input = taxestimate.TaxLineInput{Category: taxestimate.CategoryDesignConsultationFee, OrderType: taxestimate.OrderTypeJob, LineType: taxestimate.LineTypeConsultationFee, Amount: l.Amount}
		case "", "product", "install", "installation", "labor":
			cat, ok := categoryFromString(l.Category)
			if !ok {
				return nil, fmt.Errorf("line %d (%s): unknown category %q (use blinds, shutters, or draperies)", i+1, name, l.Category)
			}
			lineType := productLineType
			if k := strings.ToLower(strings.TrimSpace(l.Kind)); k == "install" || k == "installation" || k == "labor" {
				lineType = taxestimate.LineTypeAdditionalLabor
				kind = "install"
			}
			input = taxestimate.TaxLineInput{Category: cat, OrderType: taxestimate.OrderTypeJob, LineType: lineType, Amount: l.Amount}
		default:
			return nil, fmt.Errorf("line %d (%s): unknown kind %q (use product, install, or consult)", i+1, name, l.Kind)
		}
		out = append(out, builtLine{input: input, name: name, kind: kind})
	}

	if req.MeasureFee > 0 {
		out = append(out, builtLine{
			input: taxestimate.TaxLineInput{Category: taxestimate.CategoryDesignConsultationFee, OrderType: taxestimate.OrderTypeJob, LineType: taxestimate.LineTypeConsultationFee, Amount: req.MeasureFee},
			name:  "Measure / Design Consultation Fee",
			kind:  "consult",
		})
	}
	return out, nil
}

// Estimate computes the tax estimate for an entered quote. Input problems return
// an error (surfaced as 400); missing rate or unmapped lines are non-blocking
// warnings on the result.
func (e *Estimator) Estimate(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.State) == "" {
		return Result{}, fmt.Errorf("state or province is required")
	}
	if len(req.Lines) == 0 {
		return Result{}, fmt.Errorf("at least one line item is required")
	}
	if req.RateOverride < 0 {
		return Result{}, fmt.Errorf("rateOverride must be a non-negative fraction (e.g. 0.0825)")
	}

	if province, ok := e.canada.ResolveProvince(req.State); ok {
		return e.estimateCanada(req, province)
	}
	return e.estimateUS(ctx, req)
}

func (e *Estimator) estimateUS(ctx context.Context, req Request) (Result, error) {
	built, err := buildLines(req, taxestimate.LineTypeInstalledPackage)
	if err != nil {
		return Result{}, err
	}
	state, stateOK := stateFor(req.State)
	res := Result{Country: "US", State: state, Zip: strings.TrimSpace(req.Zip)}
	if !stateOK {
		res.Warnings = append(res.Warnings, "shipping location has no resolvable state; tax not estimated")
	}

	var rate taxestimate.RateResult
	if req.RateOverride > 0 {
		rate = taxestimate.RateResult{Zip: req.Zip, CombinedRate: req.RateOverride, Jurisdictions: "manual override"}
		res.RateOverridden = true
	} else {
		// Prefer a live TaxJar lookup when configured; otherwise, or when it returns
		// nothing usable, fall back to the static per-state average combined rate so
		// the estimate works with no API. The rep can always enter an exact override.
		if e.rates != nil && strings.TrimSpace(req.Zip) != "" {
			rate = e.rates.LookupRate(ctx, req.Zip)
		}
		// Offline ZIP-level rate from the embedded Avalara monthly table, when
		// present. More precise than the state average; used when TaxJar is not
		// configured or returned nothing.
		if rate.CombinedRate == 0 {
			if zr, ok := zipRateFor(req.Zip); ok {
				region := "Avalara monthly ZIP table"
				if zr.region != "" {
					region = zr.region + " (Avalara monthly ZIP table)"
				}
				rate = taxestimate.RateResult{Zip: req.Zip, CombinedRate: zr.combined, Jurisdictions: region}
				res.RateBoundaryRisk = zr.cityRate > 0
				res.RateCityRate = zr.cityRate
			}
		}
		if rate.CombinedRate == 0 {
			if avg, ok := stateAverageRate(state); ok {
				rate = taxestimate.RateResult{Zip: req.Zip, CombinedRate: avg, Jurisdictions: "state average combined rate (Tax Foundation 2026)", Estimated: true}
				res.Warnings = append(res.Warnings, "using the state average combined rate (estimate); enter a rate override for the exact local rate")
			} else {
				rate = taxestimate.RateResult{Zip: req.Zip, Estimated: true}
				res.Warnings = append(res.Warnings, "no rate available for this state; enter a rate override")
			}
		}
	}
	res.RateEstimated = rate.Estimated

	inputs := make([]taxestimate.TaxLineInput, len(built))
	for i, b := range built {
		inputs[i] = b.input
	}
	order := e.calc.Compute(channelFor(req.Channel), state, rate.CombinedRate, inputs)
	res.CombinedRate = rate.CombinedRate
	fillOrder(&res, built, order)
	if order.HasUnmapped {
		res.Warnings = append(res.Warnings, "one or more lines were not found in the taxability matrix")
	}
	// Labor-divergence warning: the matrix flags every line in a category where
	// install labor is taxed differently from product. That only risks an
	// under-estimate when the labor is hidden in a product/installed-package line
	// with no explicit install line of its own. When the quote already has an
	// install line for that category (RLN installs), the labor is computed with
	// the Additional Labor line type and is correct, so we suppress the warning.
	installed := map[taxestimate.Category]bool{}
	for i := range built {
		if built[i].kind == "install" {
			installed[order.Lines[i].Input.Category] = true
		}
	}
	laborRisk := false
	for i := range order.Lines {
		if order.Lines[i].LaborMayDiverge && built[i].kind != "install" && !installed[order.Lines[i].Input.Category] {
			laborRisk = true
			break
		}
	}
	if laborRisk {
		res.Warnings = append(res.Warnings, "labor is taxed differently from product in this state; add an install line for each product so install labor is estimated correctly")
	}
	return res, nil
}

func (e *Estimator) estimateCanada(req Request, province string) (Result, error) {
	built, err := buildLines(req, taxestimate.LineTypeProduct)
	if err != nil {
		return Result{}, err
	}
	res := Result{Country: "Canada", State: province}

	inputs := make([]taxestimate.TaxLineInput, len(built))
	for i, b := range built {
		inputs[i] = b.input
	}
	order := e.canada.Compute(province, inputs)
	// Effective rate (tax / retail): a flat base would mislead for BC, where the
	// drapery product line is 12% and everything else 5%.
	if order.Retail > 0 {
		res.CombinedRate = order.TotalTax / order.Retail
	}
	fillOrder(&res, built, order)
	res.Warnings = append(res.Warnings, "Canada: static province rate from the window tax chart; all lines taxable (no address lookup, no TaxJar).")
	return res, nil
}

// fillOrder copies the order totals and merges per-line outcomes into the result.
func fillOrder(res *Result, built []builtLine, order taxestimate.OrderTaxResult) {
	res.TaxableBase = order.TaxableBase
	res.Retail = order.Retail
	res.TotalTax = order.TotalTax
	res.OrderTotal = order.OrderTotal
	res.Blended = order.Blended
	res.BlendedRate = order.BlendedRate
	res.Lines = make([]LineResult, 0, len(order.Lines))
	for i, l := range order.Lines {
		lr := LineResult{
			Name:        built[i].name,
			Category:    string(l.Input.Category),
			Kind:        built[i].kind,
			LineType:    string(l.Input.LineType),
			Amount:      l.Input.Amount,
			Found:       l.Found,
			Taxable:     l.Taxable,
			AppliedRate: l.AppliedRate,
			Tax:         l.Tax,
			Warning:     built[i].warn,
		}
		res.Lines = append(res.Lines, lr)
	}
}

// stateByCode maps USPS two-letter codes to the full state names the matrix uses.
var stateByCode = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"DC": "District of Columbia", "FL": "Florida", "GA": "Georgia", "HI": "Hawaii",
	"ID": "Idaho", "IL": "Illinois", "IN": "Indiana", "IA": "Iowa",
	"KS": "Kansas", "KY": "Kentucky", "LA": "Louisiana", "ME": "Maine",
	"MD": "Maryland", "MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota",
	"MS": "Mississippi", "MO": "Missouri", "MT": "Montana", "NE": "Nebraska",
	"NV": "Nevada", "NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico",
	"NY": "New York", "NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio",
	"OK": "Oklahoma", "OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island",
	"SC": "South Carolina", "SD": "South Dakota", "TN": "Tennessee", "TX": "Texas",
	"UT": "Utah", "VT": "Vermont", "VA": "Virginia", "WA": "Washington",
	"WV": "West Virginia", "WI": "Wisconsin", "WY": "Wyoming",
}

// caProvinceByCode maps ISO codes to the province names the UI/calculator use.
var caProvinceByCode = map[string]string{
	"AB": "Alberta", "BC": "British Columbia", "MB": "Manitoba",
	"NB": "New Brunswick", "NL": "Newfoundland & Labrador",
	"NT": "Northwest Territories", "NS": "Nova Scotia", "NU": "Nunavut",
	"ON": "Ontario", "PE": "Prince Edward Island", "QC": "Quebec",
	"SK": "Saskatchewan", "YT": "Yukon Territory",
}

// ResolveRegion turns an ISO/USPS region code (as supplied by Vercel's edge geo
// headers) into the full region name the calculator expects. country is the ISO
// country code ("US" / "CA"). Unknown codes are returned unchanged.
func ResolveRegion(country, region string) string {
	r := strings.TrimSpace(region)
	if r == "" {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "CA":
		if full, ok := caProvinceByCode[strings.ToUpper(r)]; ok {
			return full
		}
	default: // US or unspecified
		if full, ok := stateByCode[strings.ToUpper(r)]; ok {
			return full
		}
	}
	return r
}

// stateFor resolves a state code or name to the matrix state name.
func stateFor(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if full, ok := stateByCode[strings.ToUpper(s)]; ok {
		return full, true
	}
	return s, true
}
