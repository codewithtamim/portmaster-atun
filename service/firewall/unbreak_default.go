//go:build !android

package firewall

import "github.com/safing/portmaster/service/network"

// unbreakMayLift always lets an unbreak match lift a filter-list block, as
// upstream does.
func unbreakMayLift(*network.Connection) bool { return true }
