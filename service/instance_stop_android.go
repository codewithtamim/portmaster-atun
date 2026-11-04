//go:build android

package service

// stopInterceptionGroup stops the TUN stack before the rest of the service.
// Otherwise shutdown leaves the VPN interface up with nothing reading packets.
// INVARIANT: interception stops first, then the service group. It takes
// tunLock like StopTun, so a StartTun from Java cannot run in between.
func stopInterceptionGroup(i *Instance) {
	if i == nil || i.serviceGroupInterception == nil {
		return
	}
	tunLock.Lock()
	defer tunLock.Unlock()

	_ = i.stopTunLocked()
}
