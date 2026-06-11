package taxestimate

import "testing"

func TestChannelForAccount(t *testing.T) {
	tests := []struct {
		name            string
		customerGroup   string
		customerChannel string
		want            Channel
		wantOK          bool
	}{
		{name: "HO maps to THD", customerGroup: "HO", customerChannel: "", want: ChannelTHD, wantOK: true},
		{name: "lowercase ho", customerGroup: "ho", customerChannel: "", want: ChannelTHD, wantOK: true},
		{name: "padded HO", customerGroup: "  HO  ", customerChannel: "anything", want: ChannelTHD, wantOK: true},
		{name: "customer_channel ignored for HO", customerGroup: "HO", customerChannel: "SOMETHING_ELSE", want: ChannelTHD, wantOK: true},
		// Unmapped: partner channels not yet confirmed
		{name: "empty", customerGroup: "", customerChannel: "", want: "", wantOK: false},
		{name: "whitespace only", customerGroup: "   ", customerChannel: "", want: "", wantOK: false},
		{name: "unconfirmed partner group", customerGroup: "DV", customerChannel: "DECORVIEW", want: "", wantOK: false},
		{name: "unknown group", customerGroup: "ZZ", customerChannel: "", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ChannelForAccount(tt.customerGroup, tt.customerChannel)
			if ok != tt.wantOK {
				t.Fatalf("ChannelForAccount(%q, %q) ok = %v, want %v", tt.customerGroup, tt.customerChannel, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("ChannelForAccount(%q, %q) = %q, want %q", tt.customerGroup, tt.customerChannel, got, tt.want)
			}
		})
	}
}
