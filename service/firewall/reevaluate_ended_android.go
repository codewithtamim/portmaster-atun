//go:build android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
)

// reevaluateEnded reports whether a connection that has already ended must
// still be decided again when the rules change.
//
// An echo request has no ports, so every echo to the same address is the same
// connection. After the first echo the record is marked ended, but the next
// echo reuses it and its old verdict. Without a new decision a block rule
// added afterwards would not apply to that address until the record expires.
func reevaluateEnded(conn *network.Connection) bool {
	if conn == nil || conn.Type != network.IPConnection {
		return false
	}
	return conn.IPProtocol == packet.ICMP || conn.IPProtocol == packet.ICMPv6
}
