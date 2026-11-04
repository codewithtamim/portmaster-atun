//go:build android

package firewall

import "github.com/safing/portmaster/service/network"

// awaitTunnelRequest has nothing to wait for on Android. Sluice receives
// sockets the kernel redirected; here atun already holds the connection and
// hands it to crew itself, see atun.handOffToSPN.
func awaitTunnelRequest(_ *network.Connection) error {
	return nil
}
