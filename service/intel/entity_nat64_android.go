//go:build android

package intel

import (
	"net"

	"github.com/safing/portmaster/base/utils/anet"
)

// MatchIP is the address rules, filter lists and location data are checked
// against. On a network that reaches IPv4 through NAT64, an app can write an
// IPv4 address as an IPv6 one inside the NAT64 prefix. Both forms are the same
// destination, so the IPv4 address inside is the one that counts.
func (e *Entity) MatchIP() net.IP {
	if v4 := anet.EmbeddedIPv4(e.IP); v4 != nil {
		return v4
	}
	return e.IP
}
