//go:build android

package network

import (
	"github.com/safing/portmaster/service/process"
	"github.com/safing/portmaster/spn/access"
)

func bandwidthEnabled(*access.UserRecord) bool {
	return true
}

func historyFeatureAllowed(*access.UserRecord) bool {
	return true
}

func dnsRequestOwnerUnidentified(conn *Connection) bool {
	return conn.PID < 0 && conn.UID == nil
}

// skipDNSRequestSave reports whether a DNS request must not be kept for later
// attribution. A known UID is enough when the PID is unknown or the kernel.
func skipDNSRequestSave(conn *Connection) bool {
	return (conn.PID == process.UndefinedProcessID || conn.PID == process.SystemProcessID) &&
		conn.UID == nil
}

// processContextUID records the app's UID in the connection's process context.
func processContextUID(proc *process.Process) *int {
	if proc.UserID < 0 {
		return nil
	}
	uid := proc.UserID
	return &uid
}
