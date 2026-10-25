//go:build android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/process"
)

// skipUnattributedICMPScope reports whether an internet-scope deny should be
// skipped so later deciders can still allow or block the echo.
//
// Android cannot name the app that sent the echo. The owner lookup only
// answers for TCP and UDP, so the echo is the unidentified process. Denying
// it here for a profile that blocks internet would run before endpoint rules
// and the default action, and the local echo reply would never be written.
func skipUnattributedICMPScope(conn *network.Connection, pkt packet.Packet) bool {
	if conn == nil || pkt == nil || conn.Process() == nil {
		return false
	}
	if conn.Process().Pid != process.UnidentifiedProcessID {
		return false
	}

	info := pkt.Info()
	if info == nil {
		return false
	}

	switch info.Protocol {
	case packet.ICMP, packet.ICMPv6:
		return true
	default:
		return false
	}
}
