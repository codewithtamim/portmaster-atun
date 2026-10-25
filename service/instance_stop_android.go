//go:build android

package service

import "github.com/safing/portmaster/service/firewall/interception/atun"

// stopInterceptionGroup stops the TUN stack before the rest of the service.
// Otherwise shutdown leaves the VPN interface up with nothing reading packets.
// INVARIANT: interception stops first, then the service group. The TUN config
// is cleared so a later start cannot dup a recycled fd number.
func stopInterceptionGroup(i *Instance) {
	if i == nil || i.serviceGroupInterception == nil {
		return
	}
	_ = i.serviceGroupInterception.Stop()
	atun.ClearTunConfig()
}
