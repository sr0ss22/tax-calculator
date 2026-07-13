package taxestimate

import "strings"

// Channel is an RLN sales channel used to key the taxability matrix. The seed
// matrix recognizes two channels: THD and the shared partner channel.
type Channel string

const (
	// ChannelTHD is The Home Depot. The display name is "The Home Depot"; the
	// matrix keys it as "THD".
	ChannelTHD Channel = "THD"
	// ChannelPartners is the shared Decorview/DirectBuy/Macy's/JCP channel.
	ChannelPartners Channel = "Decorview/DirectBuy/Macy's/JCP"
	// ChannelBJs is the BJ's Wholesale channel (Northeast footprint, ~17 states;
	// mostly fully taxable, Michigan installed-package-only, DE/NH/VA exempt).
	ChannelBJs Channel = "BJ's"
	// ChannelSamsClub is Sam's Club. It appears only in the channel-agnostic
	// warranty-fee chart (see warranty.go), not in the new-job or service-call
	// matrix, so it carries no base matrix rows.
	ChannelSamsClub Channel = "Sams Club"
)

// customerGroupToChannel is mapping table B: SAP account customer_group to RLN
// channel. Seeded with the one confirmed value (HO -> The Home Depot). The
// remaining channels (Decorview, DirectBuy, Macy's, JCP) are pending SME or data
// confirmation of their customer_group / customer_channel values; until then they
// resolve to not-found and the caller returns a flagged, non-blocking estimate.
var customerGroupToChannel = map[string]Channel{
	"ho": ChannelTHD,
}

// ChannelForAccount resolves the RLN channel from SAP account metadata carried on
// the quote or order. customer_group is the primary key. customer_channel is
// accepted now so the signature is stable once partner channels are confirmed,
// but it is not yet used for resolution. The second return is false when no
// channel can be derived; callers return a flagged, non-blocking estimate rather
// than guessing a channel.
func ChannelForAccount(customerGroup, customerChannel string) (Channel, bool) {
	_ = customerChannel // reserved for partner-channel mappings (pending SME confirmation)
	ch, ok := customerGroupToChannel[strings.ToLower(strings.TrimSpace(customerGroup))]
	return ch, ok
}
