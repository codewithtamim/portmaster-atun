//go:build android

package firewall

import (
	"context"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/process"
	"github.com/safing/portmaster/service/profile"
)

// Android app UIDs start at 10000. Below that is the system, which stays
// allowed while the phone is locked.
const androidFirstAppUID = 10000

// pushDeliveryIdentities carry push messages for other apps. Blocking them
// while locked would silence every app's notifications, so they stay allowed
// like the system. Matched on the package or shared user id; Play Services and
// the Services Framework share "com.google.uid.shared". microG and sandboxed
// Play on GrapheneOS use the same package names.
var pushDeliveryIdentities = map[string]struct{}{
	"com.google.android.gms": {},
	"com.google.android.gsf": {},
	"com.google.uid.shared":  {},
}

// googlePushSigners are the SHA-256 digests of Google's certificates for Play
// Services and the Services Framework. Sandboxed Play is a user app, so the
// name alone is not enough: anyone can install an app under these names where
// Google's apps are missing.
var googlePushSigners = []string{
	"5f2391277b1dbd489000467e4c2fa6af802430080457dce2f618992e9dfb5402", // Play Services
	"7ce83c1b71f3d572fed04c8d40c5cb10ff75e6d87d9df6fbd53f0468c2905053", // Services Framework
}

// exemptWhileLocked reports whether the app keeps network access while the
// phone is locked: the system, and the push services other apps rely on.
func exemptWhileLocked(proc *process.Process) bool {
	if proc.UserID < androidFirstAppUID {
		return true
	}
	named := false
	for _, id := range []string{proc.ExecName, proc.UserName, proc.Path} {
		if _, ok := pushDeliveryIdentities[id]; ok {
			named = true
			break
		}
	}
	if !named {
		return false
	}
	info, err := aconn.GetAppInfo(proc.UserID)
	if err != nil {
		return false
	}
	return trustedPushApp(info)
}

// trustedPushApp reports whether an app with a push service name really is
// one: part of the system image, or signed by Google.
func trustedPushApp(info *aconn.AppInfo) bool {
	if info == nil {
		return false
	}
	if info.SystemApp {
		return true
	}
	for _, digest := range googlePushSigners {
		if info.SignedBySameDeveloper(digest) {
			return true
		}
	}
	return false
}

func checkDevicePolicy(ctx context.Context, conn *network.Connection, p *profile.LayeredProfile, pkt packet.Packet) bool {
	if conn.Inbound {
		return false
	}
	proc := conn.Process()
	if proc == nil {
		return false
	}
	// While a Wi-Fi sign-in page is up, its connectivity domains are let
	// through before the lock and mobile blocks and before rules. Otherwise
	// a strict setup leaves the phone offline on hotel or cafe Wi-Fi with no
	// way to sign in. A fully blocked app still gets no grant.
	if conn.Entity != nil && checkConnectivityDomain(ctx, conn, p, pkt) {
		return true
	}

	state := aconn.CurrentDeviceState()
	// The Wi-Fi/mobile split follows Android's metered flag, so metered
	// Wi-Fi and hotspots count as mobile. Cellular stays mobile even where a
	// carrier marks it unmetered: that is what the user means by "mobile".
	onMobile := state.Metered || state.Cellular
	onWifi := state.Wifi && !onMobile
	switch {
	case state.Locked && p.BlockOnLock() && !exemptWhileLocked(proc):
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
