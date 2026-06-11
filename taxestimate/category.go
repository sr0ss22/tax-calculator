// Package taxestimate computes an estimated quote sales tax for RLN quoting.
//
// This is a quote-time estimate only. SAP remains the system of record for the
// actual tax on a booked order. Taxability flags come from an authoritative
// per-channel matrix seeded from the channel tax charts; the combined rate comes
// from a cached provider lookup. The matrix flags are authoritative and the rate
// provider supplies the rate only.
package taxestimate

import "strings"

// Category is a taxability product category. Every install line item maps to
// one of the first three (Blinds, Shutters, Draperies) via its product
// classification. CategoryDesignConsultationFee is a fourth, synthetic category
// that exists in the seed matrix but is NOT produced by CategoryForClassification:
// a design consultation fee is not an install product classification. The
// calculation layer (task 4) owns the consultation-fee path; until then those
// matrix rows are intentionally unreachable through mapping table A.
type Category string

const (
	CategoryBlinds                Category = "Blinds"
	CategoryShutters              Category = "Shutters"
	CategoryDraperies             Category = "Draperies"
	CategoryDesignConsultationFee Category = "Design Consultation Fee"
)

// classificationToCategory is mapping table A: the install-work-order product
// classification (productkind) to a taxability category. Keys are the lowercased
// classification strings persisted on work order line items. Matching is
// case-insensitive because productkind itself compares with strings.EqualFold.
var classificationToCategory = map[string]Category{
	"shutter":         CategoryShutters,
	"shutters":        CategoryShutters,
	"drapery":         CategoryDraperies,
	"blinds/shades":   CategoryBlinds,
	"shade/blind":     CategoryBlinds,
	"flatfabric/vane": CategoryBlinds,
	"vertical":        CategoryBlinds,
	"verticals":       CategoryBlinds,
	// Skylight and Valance/Cornice are treated as Blinds per the POC. This is
	// pending SME confirmation (design.md open question, task 2.4).
	"skylight":        CategoryBlinds,
	"valance/cornice": CategoryBlinds,
}

// CategoryForClassification maps an install product classification to its
// taxability category. The second return is false when the classification is
// empty or not in the mapping table; callers flag such a line for review and
// exclude it from the taxable base rather than guessing.
func CategoryForClassification(classification string) (Category, bool) {
	c, ok := classificationToCategory[strings.ToLower(strings.TrimSpace(classification))]
	return c, ok
}
