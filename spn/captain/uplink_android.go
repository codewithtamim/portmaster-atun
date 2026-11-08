//go:build android

package captain

import "github.com/safing/portmaster/base/utils/anet"

// uplinkChangedFlag is signalled when Android moves its uplink to another
// network, for example from mobile data to Wi-Fi.
var uplinkChangedFlag = anet.UplinkChangedFlag()

// uplinkChanged returns a channel that fires when the uplink moved.
//
// The home hub link rides a socket protected from our VPN, which follows the
// system default network. Once that moves, the link stops carrying data, but
// it keeps answering a ping for a few seconds and the next health check is a
// minute away, so the SPN stayed dead for one to three minutes. Reconnecting
// on the move builds the link on the new network at once.
func uplinkChanged() <-chan struct{} {
	return uplinkChangedFlag.Signal()
}

// ackUplinkChanged re-arms the flag after its signal was handled.
func ackUplinkChanged() {
	uplinkChangedFlag.Refresh()
}
