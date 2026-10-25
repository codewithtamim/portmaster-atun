//go:build android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile"
)

// denyDNSForBlockedInternet blocks lookups from an app whose internet is off.
// Unbreak and the DNS default allow must not open a row for that app.
func denyDNSForBlockedInternet(conn *network.Connection, p *profile.LayeredProfile) bool {
	if p == nil || !p.BlockScopeInternet() {
		return false
	}
	conn.Deny("Internet access blocked", profile.CfgOptionBlockScopeInternetKey)
	return true
}
