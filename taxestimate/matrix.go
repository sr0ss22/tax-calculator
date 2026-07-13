package taxestimate

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// taxDataJSON is the taxability matrix and rate seed ported from the RLN
// prototype (tax_data.json). The matrix flags are authoritative; the state and
// county rates are estimate fallbacks used only when a provider lookup is
// unavailable (see the rate service, task 3).
//
//go:embed data/tax_data.json
var taxDataJSON []byte

// OrderType is an order type as it appears in the taxability matrix. The proto
// OrderType enum is mapped onto these values by the calculation layer (task 4).
type OrderType string

const (
	OrderTypeJob          OrderType = "Job"
	OrderTypeServiceOrder OrderType = "Service Order"
)

// LineType is a line type as it appears in the taxability matrix. The matrix
// pairs specific line types with each order type: a Job carries an Installed
// Package or Additional Labor Services; a Service Order carries a Product or
// Installation Labor. The calculation layer (task 4) maps quote line items onto
// these values.
type LineType string

const (
	LineTypeInstalledPackage  LineType = "Installed Package (product+install)"
	LineTypeAdditionalLabor   LineType = "Additional Labor Services"
	LineTypeProduct           LineType = "Product"
	LineTypeInstallationLabor LineType = "Installation Labor"
)

// TransactionType distinguishes the tax chart a line is priced against. New
// installed jobs (the default, and the primary RLN quote flow) use the base
// matrix rows, which carry no transaction_type. Service calls (reselections,
// conversions, parts, and repairs) use the THD service-call chart, encoded as
// overlay rows tagged "service_call". Warranty fees are a separate flat-fee flow handled
// outside this matrix (see warranty.go). A lookup for a transaction type with no
// specific row falls back to the base (new-job) row, so a transaction type only
// changes an answer where the chart actually diverges.
type TransactionType string

const (
	// TxnNewJob is a new installed job. It is the zero value so existing callers
	// and existing seed rows (which carry no transaction_type) resolve to it and
	// keep their current behavior.
	TxnNewJob TransactionType = ""
	// TxnServiceCall is a service call: a reselection, conversion, part, or repair.
	TxnServiceCall TransactionType = "service_call"
)

// matrixRow is one row of the taxability matrix: a (channel, state, product,
// order_type, line_type) tuple and whether that combination is taxable.
type matrixRow struct {
	Channel string `json:"channel"`
	State   string `json:"state"`
	// Product holds a taxability Category value (Blinds, Shutters, or Draperies).
	// It is named "product" in the seed for historical reasons; it is not a SKU or
	// SAP product_type.
	Product   string `json:"product"`
	OrderType string `json:"order_type"`
	LineType  string `json:"line_type"`
	// TransactionType is empty for base new-job rows and "service_call" for THD
	// service-call-chart overlay rows.
	TransactionType string `json:"transaction_type,omitempty"`
	Taxable         bool   `json:"taxable"`
}

// stateRate is the population-weighted combined rate fallback for a state.
type stateRate struct {
	State    string  `json:"state"`
	Base     float64 `json:"base"`
	AvgLocal float64 `json:"avg_local"`
	Combined float64 `json:"combined"`
}

// countySeed is a verified point combined rate for a specific locality.
type countySeed struct {
	State    string  `json:"state"`
	Locality string  `json:"locality"`
	Combined float64 `json:"combined"`
	Status   string  `json:"status"`
	Note     string  `json:"note"`
}

// taxData is the on-disk shape of the seed file.
type taxData struct {
	Meta       map[string]any `json:"meta"`
	Channels   []string       `json:"channels"`
	Products   []string       `json:"products"`
	NoStateTax []string       `json:"no_state_tax"`
	Matrix     []matrixRow    `json:"matrix"`
	StateRates []stateRate    `json:"state_rates"`
	CountySeed []countySeed   `json:"county_seed"`
}

// MatrixKey identifies a single taxability decision: one channel, state,
// category, order type, and line type.
//
// State is the full English state name as it appears in the seed (for example
// "Texas", "District of Columbia"), not a two-letter code. Alaska is the one
// exception: it is split into locality-suffixed keys ("Alaska - Anchorage",
// "Alaska - Juneau", "Alaska - Kenai", "Alaska - Wasilla") with no plain
// "Alaska" row, because Alaska has no state tax and taxability is local. The
// calculation layer (task 4) must map a quote ZIP or state onto these exact
// strings, including resolving an Alaska ZIP to its locality, or the lookup
// returns not-found.
type MatrixKey struct {
	Channel         Channel
	State           string
	Category        Category
	OrderType       OrderType
	LineType        LineType
	TransactionType TransactionType
}

// Matrix is the authoritative taxability matrix loaded from the seed data. Its
// flags decide whether a line is taxable; the rate provider supplies only the
// rate.
type Matrix struct {
	index map[MatrixKey]bool
}

var (
	defaultMatrix *Matrix
	defaultErr    error
	defaultOnce   sync.Once
)

// Default returns a process-wide memoized Matrix. The 248KB seed is parsed and
// indexed once on first call. Callers serving quote-time lookups (high volume)
// SHOULD use Default rather than LoadMatrix so they do not re-parse the seed per
// request.
func Default() (*Matrix, error) {
	defaultOnce.Do(func() {
		defaultMatrix, defaultErr = loadMatrix(taxDataJSON)
	})
	return defaultMatrix, defaultErr
}

// LoadMatrix parses the embedded seed data into a fresh, indexed taxability
// matrix. Prefer Default for request-path use; LoadMatrix is for callers that
// explicitly want an independent instance (and for tests).
func LoadMatrix() (*Matrix, error) {
	return loadMatrix(taxDataJSON)
}

// knownCategories is the set of category values the matrix is allowed to carry.
var knownCategories = map[Category]bool{
	CategoryBlinds:    true,
	CategoryShutters:  true,
	CategoryDraperies: true,
}

// knownChannels, knownOrderTypes, and knownLineTypes are the value sets the seed
// is allowed to carry for those columns. Like knownCategories they let loadMatrix
// fail fast when the seed introduces a typo or a new value the type system cannot
// represent, instead of casting the raw string into a typed key that no runtime
// lookup will ever match (which would silently treat the line as unmapped and
// under-estimate tax).
var knownChannels = map[Channel]bool{
	ChannelTHD:      true,
	ChannelPartners: true,
	ChannelBJs:      true,
	ChannelSamsClub: true,
}

var knownOrderTypes = map[OrderType]bool{
	OrderTypeJob:          true,
	OrderTypeServiceOrder: true,
}

var knownLineTypes = map[LineType]bool{
	LineTypeInstalledPackage:  true,
	LineTypeAdditionalLabor:   true,
	LineTypeProduct:           true,
	LineTypeInstallationLabor: true,
}

var knownTransactionTypes = map[TransactionType]bool{
	TxnNewJob:      true,
	TxnServiceCall: true,
}

// loadMatrix parses raw seed bytes into an indexed matrix. It fails loudly so a
// broken or empty seed does not silently treat every line as untaxed, rejects an
// unknown product so a re-port that introduces a category the type system cannot
// represent is caught at startup, and rejects a duplicate key with a conflicting
// taxable flag so an accidental double-entry cannot silently overwrite.
func loadMatrix(raw []byte) (*Matrix, error) {
	var data taxData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("taxestimate: parse seed data: %w", err)
	}
	if len(data.Matrix) == 0 {
		return nil, fmt.Errorf("taxestimate: seed matrix is empty")
	}
	index := make(map[MatrixKey]bool, len(data.Matrix))
	for i, r := range data.Matrix {
		category := Category(r.Product)
		if !knownCategories[category] {
			return nil, fmt.Errorf("taxestimate: row %d has unknown product %q", i, r.Product)
		}
		channel := Channel(r.Channel)
		if !knownChannels[channel] {
			return nil, fmt.Errorf("taxestimate: row %d has unknown channel %q", i, r.Channel)
		}
		orderType := OrderType(r.OrderType)
		if !knownOrderTypes[orderType] {
			return nil, fmt.Errorf("taxestimate: row %d has unknown order_type %q", i, r.OrderType)
		}
		lineType := LineType(r.LineType)
		if !knownLineTypes[lineType] {
			return nil, fmt.Errorf("taxestimate: row %d has unknown line_type %q", i, r.LineType)
		}
		txnType := TransactionType(r.TransactionType)
		if !knownTransactionTypes[txnType] {
			return nil, fmt.Errorf("taxestimate: row %d has unknown transaction_type %q", i, r.TransactionType)
		}
		key := MatrixKey{
			Channel:         channel,
			State:           r.State,
			Category:        category,
			OrderType:       orderType,
			LineType:        lineType,
			TransactionType: txnType,
		}
		if existing, dup := index[key]; dup && existing != r.Taxable {
			return nil, fmt.Errorf("taxestimate: row %d duplicates key %+v with a conflicting taxable flag", i, key)
		}
		index[key] = r.Taxable
	}
	return &Matrix{index: index}, nil
}

// Taxable returns whether the keyed line is taxable. The second return is false
// when the key is not present in the matrix; callers flag such a line for review
// and exclude it from the taxable base rather than assuming a default.
func (m *Matrix) Taxable(key MatrixKey) (taxable bool, found bool) {
	if t, ok := m.index[key]; ok {
		return t, true
	}
	// Fall back to the base (new-job) row when no transaction-type-specific row
	// exists, so a transaction type only changes an answer where the chart
	// actually diverges; every other line keeps its new-job treatment.
	if key.TransactionType != TxnNewJob {
		base := key
		base.TransactionType = TxnNewJob
		if t, ok := m.index[base]; ok {
			return t, true
		}
	}
	return false, false
}

// Len returns the number of distinct rows indexed.
func (m *Matrix) Len() int { return len(m.index) }
