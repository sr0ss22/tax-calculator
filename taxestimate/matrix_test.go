package taxestimate

import (
	"sort"
	"strings"
	"testing"
)

func TestLoadMatrix(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	// The ported RLN matrix has 1352 unique (channel, state, product, order_type,
	// line_type) rows. A drift here means the seed changed; if the change was
	// intentional, update this number, otherwise investigate the regression.
	// 2026-06-11 (RDIS-135): 1300 -> 1352. Added 52 THD Design Consultation Fee
	// rows (one per state/locality). The THD chart has no consult-fee column, so
	// taxability is derived from THD Additional Labor Services for the state and is
	// pending SME confirmation.
	if got := m.Len(); got != 1352 {
		t.Errorf("Matrix.Len() = %d, want 1352 (update only if the seed change was intentional)", got)
	}
}

func TestDefault_Memoized(t *testing.T) {
	m1, err := Default()
	if err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	m2, err := Default()
	if err != nil {
		t.Fatalf("Default() second call error = %v", err)
	}
	if m1 != m2 {
		t.Errorf("Default() returned distinct instances; expected the same memoized pointer")
	}
}

func TestMatrix_Taxable(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}

	tests := []struct {
		name        string
		key         MatrixKey
		wantTaxable bool
		wantFound   bool
	}{
		{
			name:        "THD Texas Blinds installed is taxable",
			key:         MatrixKey{Channel: ChannelTHD, State: "Texas", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantTaxable: true,
			wantFound:   true,
		},
		{
			name:        "THD Texas Shutters installed is exempt",
			key:         MatrixKey{Channel: ChannelTHD, State: "Texas", Category: CategoryShutters, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantTaxable: false,
			wantFound:   true,
		},
		{
			name:        "THD Arkansas Blinds installed is not taxable",
			key:         MatrixKey{Channel: ChannelTHD, State: "Arkansas", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantTaxable: false,
			wantFound:   true,
		},
		{
			name:        "Oregon Blinds installed is not taxable (no state tax)",
			key:         MatrixKey{Channel: ChannelTHD, State: "Oregon", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantTaxable: false,
			wantFound:   true,
		},
		{
			name:      "plain Alaska is not found (split into localities)",
			key:       MatrixKey{Channel: ChannelTHD, State: "Alaska", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantFound: false,
		},
		{
			name:        "Alaska Juneau Blinds installed is taxable (local tax)",
			key:         MatrixKey{Channel: ChannelTHD, State: "Alaska - Juneau", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantTaxable: true,
			wantFound:   true,
		},
		{
			name:      "unknown state is not found",
			key:       MatrixKey{Channel: ChannelTHD, State: "Atlantis", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantFound: false,
		},
		{
			name:      "unknown channel is not found",
			key:       MatrixKey{Channel: "BOGUS", State: "Texas", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantFound: false,
		},
		{
			name:      "two-letter state code is not found (full names only)",
			key:       MatrixKey{Channel: ChannelTHD, State: "TX", Category: CategoryBlinds, OrderType: OrderTypeJob, LineType: LineTypeInstalledPackage},
			wantFound: false,
		},
		{
			name:      "empty key is not found",
			key:       MatrixKey{},
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTaxable, gotFound := m.Taxable(tt.key)
			if gotFound != tt.wantFound {
				t.Fatalf("Taxable(%+v) found = %v, want %v", tt.key, gotFound, tt.wantFound)
			}
			if tt.wantFound && gotTaxable != tt.wantTaxable {
				t.Errorf("Taxable(%+v) taxable = %v, want %v", tt.key, gotTaxable, tt.wantTaxable)
			}
		})
	}
}

// TestMatrix_Taxable_PartnerChannel confirms the second channel ("Decorview/
// DirectBuy/Macy's/JCP") is present and queryable in the matrix.
func TestMatrix_Taxable_PartnerChannel(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	key := MatrixKey{
		Channel:   ChannelPartners,
		State:     "Texas",
		Category:  CategoryBlinds,
		OrderType: OrderTypeJob,
		LineType:  LineTypeInstalledPackage,
	}
	if _, found := m.Taxable(key); !found {
		t.Errorf("partner channel row %+v not found in matrix", key)
	}
}

// TestMatrix_ConsultationFeeReachable documents that the synthetic Design
// Consultation Fee rows exist in the seed (so the data is not lost) but are NOT
// reachable through mapping table A. The calculation layer (task 4) owns wiring
// a consultation-fee line to this category.
func TestMatrix_ConsultationFeeReachable(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	key := MatrixKey{
		Channel:   ChannelPartners,
		State:     "Alaska - Juneau",
		Category:  CategoryDesignConsultationFee,
		OrderType: OrderTypeJob,
		LineType:  LineTypeConsultationFee,
	}
	if _, found := m.Taxable(key); !found {
		t.Errorf("Design Consultation Fee row %+v not found in matrix", key)
	}
	// Mapping table A must never yield the consultation-fee category.
	if _, ok := CategoryForClassification("Design Consultation Fee"); ok {
		t.Errorf("CategoryForClassification should not map a consultation fee to a category")
	}
}

// TestMatrix_StateSet locks the set of distinct state keys, including the Alaska
// locality split and District of Columbia. A drift forces an explicit decision.
func TestMatrix_StateSet(t *testing.T) {
	m, err := LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix() error = %v", err)
	}
	states := map[string]bool{}
	for key := range m.index {
		states[key.State] = true
	}
	if len(states) != 54 {
		got := make([]string, 0, len(states))
		for s := range states {
			got = append(got, s)
		}
		sort.Strings(got)
		t.Fatalf("distinct states = %d, want 54 (50 states + DC + 4 Alaska localities - plain Alaska). got: %v", len(states), got)
	}
	for _, locality := range []string{"Alaska - Anchorage", "Alaska - Juneau", "Alaska - Kenai", "Alaska - Wasilla"} {
		if !states[locality] {
			t.Errorf("expected Alaska locality %q in state set", locality)
		}
	}
	if states["Alaska"] {
		t.Errorf("plain %q should not be a state key; Alaska is split into localities", "Alaska")
	}
	if !states["District of Columbia"] {
		t.Errorf("expected %q in state set", "District of Columbia")
	}
}

func TestLoadMatrix_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "malformed json", raw: `{`, wantErr: "parse seed data"},
		{name: "empty matrix", raw: `{"matrix":[]}`, wantErr: "seed matrix is empty"},
		{
			name:    "unknown product",
			raw:     `{"matrix":[{"channel":"THD","state":"Texas","product":"Rugs","order_type":"Job","line_type":"Product","taxable":true}]}`,
			wantErr: "unknown product",
		},
		{
			name:    "unknown channel",
			raw:     `{"matrix":[{"channel":"Wayfair","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Product","taxable":true}]}`,
			wantErr: "unknown channel",
		},
		{
			name:    "unknown order_type",
			raw:     `{"matrix":[{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Lease","line_type":"Product","taxable":true}]}`,
			wantErr: "unknown order_type",
		},
		{
			name:    "unknown line_type",
			raw:     `{"matrix":[{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Freight","taxable":true}]}`,
			wantErr: "unknown line_type",
		},
		{
			name: "duplicate key with conflicting flag",
			raw: `{"matrix":[` +
				`{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Product","taxable":true},` +
				`{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Product","taxable":false}` +
				`]}`,
			wantErr: "conflicting taxable flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadMatrix([]byte(tt.raw))
			if err == nil {
				t.Fatalf("loadMatrix(%q) error = nil, want error containing %q", tt.raw, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("loadMatrix error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestLoadMatrix_DuplicateKeySameFlagAllowed confirms a benign exact-duplicate
// (same key, same flag) does not error; only a conflicting flag is rejected.
func TestLoadMatrix_DuplicateKeySameFlagAllowed(t *testing.T) {
	raw := `{"matrix":[` +
		`{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Product","taxable":true},` +
		`{"channel":"THD","state":"Texas","product":"Blinds","order_type":"Job","line_type":"Product","taxable":true}` +
		`]}`
	m, err := loadMatrix([]byte(raw))
	if err != nil {
		t.Fatalf("loadMatrix() error = %v, want nil for an exact duplicate", err)
	}
	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (exact duplicate collapses)", m.Len())
	}
}
