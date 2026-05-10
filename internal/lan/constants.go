package lan

import "time"

const (
	MulticastAddr   = "224.0.0.123:9101"
	BroadcastAddr   = "255.255.255.255:9101"
	DefaultHTTPPort = 19100
	MaxDatagramSize = 8192
	PeerTTL         = 30 * time.Second

	HTTPDiscoveryInterval    = 15 * time.Second
	HTTPDiscoveryTimeout     = 2 * time.Second
	SummaryCacheTTL          = 20 * time.Second
	httpDiscoveryConcurrency = 32
)
