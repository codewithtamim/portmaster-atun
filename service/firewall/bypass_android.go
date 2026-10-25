//go:build android

package firewall

import (
	"github.com/safing/portmaster/service/compat"
	"github.com/safing/portmaster/service/network"
)

// reportUnblockedSecureDNS warns when an app uses its own secure DNS while
// blocking it is off, the Android default. The connection goes
// through and the warning says its lookups skip filtering. Without
// this, detection would only happen inside PreventBypassing, which runs only
// when blocking is on, so the default setting would fail silently.
func reportUnblockedSecureDNS(conn *network.Connection) {
	if conn.Inbound || conn.Type == network.DNSRequest {
		return
	}
	proc := conn.Process()
	// System UIDs are skipped: Android's own Private DNS travels as the
	// system, and it has its own warning.
	if proc == nil || proc.UserID < androidFirstAppUID || proc.IsSystemResolver() {
		return
	}
	if conn.Entity.Port == 853 || conn.Entity.MatchLists(resolverFilterLists) {
		compat.ReportSecureDNSSkipped(proc)
	}
}
