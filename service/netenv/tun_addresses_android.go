//go:build android

package netenv

import "net"

// The VpnService TUN takes 100.127.247.245 unless the current network
// already uses that range, in which case it falls back to an address in the
// benchmarking range 198.18.0.0/15. Neither is a real uplink, so both are
// ignored by the online check. Set at init, before any check runs.
func init() {
	IgnoreIPsInOnlineStatusCheck = append(IgnoreIPsInOnlineStatusCheck, net.IPv4(198, 18, 247, 245))
}
