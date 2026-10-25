//go:build android

package crew

// releaseAbandonedTunnel closes the app connection when SPN setup fails before
// the connect op takes ownership. Otherwise the TUN flow, endpoint and socket
// stay open until the VPN stops.
func releaseAbandonedTunnel(t *Tunnel) {
	if t == nil || t.conn == nil {
		return
	}
	_ = t.conn.Close()
}
