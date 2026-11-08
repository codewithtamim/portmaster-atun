//go:build android

package network

// ConnectionStillOpen reports whether the core has the connection with this
// ID and it has not ended. netquery uses it to find rows whose end was lost.
func ConnectionStillOpen(connID string) bool {
	conn, ok := GetConnection(connID)
	if !ok {
		return false
	}
	conn.Lock()
	defer conn.Unlock()
	return conn.Ended == 0
}
