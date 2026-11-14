//go:build android

package netenv

import "net"

// The VpnService TUN takes 100.127.247.245 unless the current network
// already uses that range. It then falls back to 198.18.247.245 (the
// benchmarking range 198.18.0.0/15) and last to 172.31.247.245. None of them
// is a real uplink, so all are ignored by the online check. Upstream already
// lists the first; the fallbacks are added here. Set at init, before any
// check runs.
func init() {
	IgnoreIPsInOnlineStatusCheck = append(IgnoreIPsInOnlineStatusCheck,
		net.IPv4(198, 18, 247, 245),
		net.IPv4(172, 31, 247, 245),
	)
}
