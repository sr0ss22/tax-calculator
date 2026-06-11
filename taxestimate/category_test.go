package taxestimate

import "testing"

func TestCategoryForClassification(t *testing.T) {
	tests := []struct {
		name           string
		classification string
		want           Category
		wantOK         bool
	}{
		// Shutters
		{name: "Shutter", classification: "Shutter", want: CategoryShutters, wantOK: true},
		{name: "Shutters", classification: "Shutters", want: CategoryShutters, wantOK: true},
		// Draperies
		{name: "Drapery", classification: "Drapery", want: CategoryDraperies, wantOK: true},
		// Blinds (the seven that fold into Blinds)
		{name: "Blinds/Shades", classification: "Blinds/Shades", want: CategoryBlinds, wantOK: true},
		{name: "Shade/Blind", classification: "Shade/Blind", want: CategoryBlinds, wantOK: true},
		{name: "FlatFabric/Vane", classification: "FlatFabric/Vane", want: CategoryBlinds, wantOK: true},
		{name: "Vertical", classification: "Vertical", want: CategoryBlinds, wantOK: true},
		{name: "Verticals", classification: "Verticals", want: CategoryBlinds, wantOK: true},
		{name: "Skylight", classification: "Skylight", want: CategoryBlinds, wantOK: true},
		{name: "Valance/Cornice", classification: "Valance/Cornice", want: CategoryBlinds, wantOK: true},
		// Case-insensitive and whitespace tolerant
		{name: "lowercase", classification: "shutters", want: CategoryShutters, wantOK: true},
		{name: "uppercase", classification: "DRAPERY", want: CategoryDraperies, wantOK: true},
		{name: "padded whitespace", classification: "  Blinds/Shades  ", want: CategoryBlinds, wantOK: true},
		// Edge cases: not mapped
		{name: "empty", classification: "", want: "", wantOK: false},
		{name: "whitespace only", classification: "   ", want: "", wantOK: false},
		{name: "unknown", classification: "Awning", want: "", wantOK: false},
		{name: "consultation fee not a classification", classification: "Design Consultation Fee", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CategoryForClassification(tt.classification)
			if ok != tt.wantOK {
				t.Fatalf("CategoryForClassification(%q) ok = %v, want %v", tt.classification, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("CategoryForClassification(%q) = %q, want %q", tt.classification, got, tt.want)
			}
		})
	}
}

// TestCategoryForClassification_CoversAllProductkindValues locks the contract that
// every product classification productkind recognizes maps to a category. If
// install-work-order adds a new classification, this test forces a mapping decision.
func TestCategoryForClassification_CoversAllProductkindValues(t *testing.T) {
	// Mirror of productkind.productClassificationValues (services/install-work-order).
	// Duplicated intentionally: importing the install service would add cross-service
	// coupling. If that list changes, this test should be updated in lockstep.
	productkindValues := []string{
		"Blinds/Shades",
		"Drapery",
		"FlatFabric/Vane",
		"Shade/Blind",
		"Shutter",
		"Shutters",
		"Skylight",
		"Valance/Cornice",
		"Vertical",
		"Verticals",
	}
	for _, v := range productkindValues {
		if _, ok := CategoryForClassification(v); !ok {
			t.Errorf("classification %q has no taxability category mapping", v)
		}
	}
}
