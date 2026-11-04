//go:build !android

package firewall

import (
	"context"
	"net"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/spn/captain"
)

// routeActive reports whether any routing target wants connections.
// The SPN is the only target here.
func routeActive() bool {
	return tunnelEnabled()
}

// bypassOwnConnection reports whether one of Portmaster's own connections must
// skip tunneling. The SPN bootstraps through its own sockets until the captain
// is ready.
func bypassOwnConnection(ip net.IP) bool {
	if !captain.ClientReady() {
		return true
	}
	return captain.IsExcepted(ip)
}

// checkAlternateRouting reports whether a non-SPN routing target decided this
// connection. There is no such target on this platform, so the SPN path in
// checkTunneling runs exactly as it always has.
func checkAlternateRouting(_ context.Context, _ *network.Connection) bool {
	return false
}
