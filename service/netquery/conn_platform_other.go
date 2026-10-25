//go:build !android

package netquery

import (
	"net"

	"github.com/safing/portmaster/service/network"
)

func storedRemoteIP(ip net.IP) string {
	return ip.String()
}

// addPlatformExtraData adds nothing: desktop history rows have no
// platform extra_data.
func addPlatformExtraData(map[string]interface{}, *network.Connection) {}
