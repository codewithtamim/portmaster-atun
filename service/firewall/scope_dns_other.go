//go:build !android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile"
)

func denyDNSForBlockedInternet(_ *network.Connection, _ *profile.LayeredProfile) bool {
	return false
}
