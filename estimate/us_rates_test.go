package estimate

import "testing"

func TestStateAverageRate(t *testing.T) {
	if got := len(stateAverageCombinedRate); got != 51 {
		t.Errorf("rate table has %d entries, want 51 (50 states + DC)", got)
	}
	tests := []struct {
		state  string
		want   float64
		wantOK bool
	}{
		{"Texas", 0.0820, true},
		{"California", 0.0899, true},
		{"Louisiana", 0.1011, true},
		{"District of Columbia", 0.0600, true},
		{"Oregon", 0.0000, true},    // no sales tax, present and zero
		{"Atlantis", 0.0000, false}, // not a real state
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got, ok := stateAverageRate(tt.state)
			if ok != tt.wantOK || (ok && !approxEq(got, tt.want)) {
				t.Errorf("stateAverageRate(%q) = (%v, %v), want (%v, %v)", tt.state, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
