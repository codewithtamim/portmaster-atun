//go:build android

package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
)

// Desktop split-tunnel proxies do not run in the VpnService process.
func checkSplitTunneling(_ context.Context, _ *network.Connection) {}

func requestSplitTunneling(_ context.Context, _ *network.Connection) error {
	return nil
}

func isOwnSplitTunnelProxyConnection(_ *network.Connection) bool {
	return false
}
