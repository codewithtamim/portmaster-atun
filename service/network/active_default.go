//go:build !android

package network

import (
	"time"

	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/network/state"
	"github.com/safing/portmaster/service/process"
)

// connectionStillActive reports whether the OS still has this TCP or UDP socket.
func connectionStillActive(conn *Connection, now time.Time) bool {
	return state.Exists(&packet.Info{
		Inbound:  false, // src == local
		Version:  conn.IPVersion,
		Protocol: conn.IPProtocol,
		Src:      conn.LocalIP,
		SrcPort:  conn.LocalPort,
		Dst:      conn.Entity.IP,
		DstPort:  conn.Entity.Port,
		PID:      process.UndefinedProcessID,
		SeenAt:   time.Unix(conn.Started, 0), // State tables will be updated if older than this.
	}, now)
}
