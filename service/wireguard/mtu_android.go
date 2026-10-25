//go:build android

package wireguard

import (
	"net"

	"github.com/safing/portmaster/base/utils/anet"
)

// tunnelMTU is the MTU the device runs with. The engine owns it: a
// config's MTU entry never overrides it.
func tunnelMTU() int {
	ifaces, err := anet.Interfaces()
	if err != nil {
		return maxTunnelMTU
	}
	return tunnelMTUFor(ifaces)
}

// tunnelMTUFor fits the tunnel inside the smallest uplink, so a tunnel packet
// never needs fragmenting on any network the phone may switch to. Loopback
// and VPN (point-to-point) interfaces are not uplinks. The result stays
// within [minMTU, maxTunnelMTU]; without a known uplink it is maxTunnelMTU,
// wg-quick's default.
func tunnelMTUFor(ifaces []net.Interface) int {
	link := 0
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 ||
			iface.MTU <= 0 {
			continue
		}
		if link == 0 || iface.MTU < link {
			link = iface.MTU
		}
	}
	if link == 0 {
		return maxTunnelMTU
	}
	return min(max(link-wireGuardOverhead, minMTU), maxTunnelMTU)
}
