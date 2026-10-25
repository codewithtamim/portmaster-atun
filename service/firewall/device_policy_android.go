//go:build android

package firewall

import (
	"context"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/profile"
)

// Android app UIDs start at 10000. Below that is the system, which stays
// allowed while the phone is locked.
const androidFirstAppUID = 10000

func checkDevicePolicy(ctx context.Context, conn *network.Connection, p *profile.LayeredProfile, _ packet.Packet) bool {
	if conn.Inbound {
		return false
	}
	proc := conn.Process()
	if proc == nil {
		return false
	}

	state := aconn.CurrentDeviceState()
	// The Wi-Fi/mobile split follows Android's metered flag, so metered
	// Wi-Fi and hotspots count as mobile. Cellular stays mobile even where a
	// carrier marks it unmetered: that is what the user means by "mobile".
	onMobile := state.Metered || state.Cellular
	onWifi := state.Wifi && !onMobile
	switch {
	case state.Locked && p.BlockOnLock() && proc.UserID >= androidFirstAppUID:
		log.Tracer(ctx).Infof("filter: blocking %s while the phone is locked", conn)
		conn.Block("blocked while the phone is locked", profile.CfgOptionBlockOnLockKey)
		return true
	case onMobile && p.BlockOnMobile():
		log.Tracer(ctx).Infof("filter: blocking %s on mobile data", conn)
		conn.Block("blocked on mobile data", profile.CfgOptionBlockOnMobileKey)
		return true
	case onWifi && p.BlockOnWifi():
		log.Tracer(ctx).Infof("filter: blocking %s on Wi-Fi", conn)
		conn.Block("blocked on Wi-Fi", profile.CfgOptionBlockOnWifiKey)
		return true
	default:
		return false
	}
}
