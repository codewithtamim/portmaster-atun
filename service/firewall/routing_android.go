//go:build android

package firewall

import (
	"context"
	"net"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/routing"
	"github.com/safing/portmaster/service/wireguard"
	"github.com/safing/portmaster/spn/captain"
)

// routeActive reports whether any routing target wants connections.
func routeActive() bool {
	switch routing.Current() {
	case routing.TargetSPN:
		return tunnelEnabled()
	case routing.TargetWireGuard:
		return true
	case routing.TargetDirect:
		return false
	default:
		return false
	}
}

// bypassOwnConnection reports whether one of Portmaster's own connections must
// skip tunneling.
//
// For WireGuard that is only the peer endpoint itself: the handshake rides a
// protected socket, and routing it into the tunnel would make it travel through
// itself. Everything else we originate can and should be tunneled.
func bypassOwnConnection(ip net.IP) bool {
	if routing.Current() == routing.TargetWireGuard {
		return wireguard.IsEndpoint(ip)
	}
	if !captain.ClientReady() {
		return true
	}
	return captain.IsExcepted(ip)
}

// checkAlternateRouting decides a connection when the routing target is not the
// SPN. It returns true when it decided, which stops checkTunneling.
func checkAlternateRouting(ctx context.Context, conn *network.Connection) bool {
	if routing.Current() != routing.TargetWireGuard {
		return false
	}

	// No imported config is not a tunnel yet. Blocking the phone before
	// the user has a file would look like a failure, and the routing
	// screen shows a direct connection until one is imported. A selected
	// config whose device is down still fails closed below.
	if wireguard.Status().Profile.Name == "" {
		return true
	}

	// Direct is the same overlay for both tunnels: spn/use=false. Hop
	// policy stays SPN-only, but skipping the tunnel applies here too.
	if layered := conn.Process().Profile(); layered != nil && !layered.UseSPN() {
		return true
	}

	switch wireguard.RouteFor(conn.Entity.IP) {
	case wireguard.RouteTunnel:
		// SetVerdict rather than SetVerdictDirectly: the reason and the option
		// key are what let the UI say why a connection took this route and
		// deep-link to the setting that caused it.
		conn.SetVerdict(
			network.VerdictRerouteToWireGuard,
			"tunneling via WireGuard",
			routing.CfgOptionTargetKey,
			nil,
		)
		conn.Tunneled = true
		// Stored as TunnelContext so the existing save path keeps it in
		// extra_data. A new connection field would change the desktop record.
		// StopTunnel is a no-op: this is not an SPN tunnel to tear down.
		configName, host, address, country := wireguard.ActivePeer()
		conn.TunnelContext = &network.WireGuardRoute{
			Kind:       "wireguard",
			ConfigName: configName,
			Country:    country,
			Endpoint:   host,
			Address:    address,
		}
		conn.SaveWhenFinished()

	case wireguard.RouteBlockFamily:
		// The tunnel is up but cannot carry this family. Sending it out
		// directly would leak it, so block it and let the app use the other.
		family := "IPv6"
		if conn.Entity.IP.To4() != nil {
			family = "IPv4"
		}
		conn.Block("WireGuard config does not carry "+family, routing.CfgOptionTargetKey)

	case wireguard.RouteBlock:
		// Fail closed. The tunnel is the route the user chose; letting the
		// connection out directly while it is down would leak it.
		log.Tracer(ctx).Trace("firewall: wireguard tunnel is not connected")
		conn.Block("WireGuard tunnel is not connected", routing.CfgOptionTargetKey)
	}

	return true
}
