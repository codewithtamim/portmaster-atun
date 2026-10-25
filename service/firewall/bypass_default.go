//go:build !android

package firewall

import "github.com/safing/portmaster/service/network"

// reportUnblockedSecureDNS does nothing: desktop reports secure DNS only when
// it blocks it.
func reportUnblockedSecureDNS(*network.Connection) {}
