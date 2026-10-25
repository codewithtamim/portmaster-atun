//go:build !android

package network

import (
	"github.com/safing/portmaster/service/process"
	"github.com/safing/portmaster/spn/access"
	"github.com/safing/portmaster/spn/access/account"
)

func bandwidthEnabled(user *access.UserRecord) bool {
	return user.MayUse(account.FeatureBWVis)
}

func historyFeatureAllowed(user *access.UserRecord) bool {
	return user.MayUse(account.FeatureHistory)
}

func dnsRequestOwnerUnidentified(conn *Connection) bool {
	return conn.PID < 0
}

// skipDNSRequestSave reports whether a DNS request must not be kept for later
// attribution. Unknown and kernel PIDs are skipped here regardless of UID.
func skipDNSRequestSave(conn *Connection) bool {
	return conn.PID == process.UndefinedProcessID || conn.PID == process.SystemProcessID
}

// processContextUID is nil on desktop, so records carry no UID.
func processContextUID(*process.Process) *int {
	return nil
}
