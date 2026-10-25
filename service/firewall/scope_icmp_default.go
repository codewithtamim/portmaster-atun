//go:build !android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
)

// skipUnattributedICMPScope keeps the internet-scope deny: outside Android,
// the interception can name the ICMP sender.
func skipUnattributedICMPScope(*network.Connection, packet.Packet) bool {
	return false
}
