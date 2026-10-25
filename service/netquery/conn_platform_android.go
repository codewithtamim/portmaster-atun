//go:build android

package netquery

import (
	"net"

	"github.com/safing/portmaster/service/network"
)

// storedRemoteIP omits a missing address. net.IP(nil).String() is "<nil>",
// which the monitor then showed as the connection address.
func storedRemoteIP(ip net.IP) string {
	if len(ip) == 0 {
		return ""
	}
	return ip.String()
}

// addPlatformExtraData records the owning app's UID. Android apps have no
// usable PID, so the UI groups and opens history rows by UID instead.
func addPlatformExtraData(extraData map[string]interface{}, conn *network.Connection) {
	if conn.ProcessContext.UID != nil {
		extraData["uid"] = *conn.ProcessContext.UID
	}
}
