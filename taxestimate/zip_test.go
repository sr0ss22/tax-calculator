package taxestimate

import "testing"

func TestNormalizeZip(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{name: "plain zip5", raw: "77002", want: "77002", wantOK: true},
		{name: "zip plus four truncated", raw: "77002-1234", want: "77002", wantOK: true},
		{name: "leading zeros preserved", raw: "01001", want: "01001", wantOK: true},
		{name: "surrounding whitespace trimmed", raw: "  90210 ", want: "90210", wantOK: true},
		{name: "empty", raw: "", want: "", wantOK: false},
		{name: "whitespace only", raw: "   ", want: "", wantOK: false},
		{name: "too short", raw: "7700", want: "", wantOK: false},
		{name: "too long without dash", raw: "770021", want: "", wantOK: false},
		{name: "letters", raw: "ABCDE", want: "", wantOK: false},
		{name: "alphanumeric postal code", raw: "K1A0B1", want: "", wantOK: false},
		{name: "malformed plus four", raw: "77002-12", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeZip(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("NormalizeZip(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("NormalizeZip(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
