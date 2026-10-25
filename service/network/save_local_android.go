//go:build android

package network

// skipLocalNetworkSave drops LAN and localhost flows from history.
// DNS requests and port 53 stay, so lookups are still recorded.
func skipLocalNetworkSave(conn *Connection) bool {
	if conn == nil || conn.Type == DNSRequest {
		return false
	}
	if conn.Entity == nil {
		return false
	}
	if conn.Entity.Port == 53 {
		return false
	}
	return conn.Entity.IPScope.IsLAN() || conn.Entity.IPScope.IsLocalhost()
}
