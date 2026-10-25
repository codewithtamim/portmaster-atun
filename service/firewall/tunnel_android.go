//go:build android

package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
)

// awaitTunnelRequest has nothing to wait for on Android. Sluice receives
// sockets the kernel redirected; here atun already holds the connection and
// hands it to crew itself, see atun.handOffToSPN.
func awaitTunnelRequest(_ *network.Connection) error {
	return nil
}

// Desktop split-tunnel proxies do not run in the VpnService process.
func checkSplitTunneling(_ context.Context, _ *network.Connection) {}

func requestSplitTunneling(_ context.Context, _ *network.Connection) error {
	return nil
}

func isOwnSplitTunnelProxyConnection(_ *network.Connection) bool {
	return false
}
