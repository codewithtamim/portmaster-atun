//go:build android

package netenv

import "net"

// Nameservers returns nothing on Android. Apps are pointed at the fake DNS
// address on the TUN; the real resolver is Portmaster itself.
func Nameservers() []Nameserver {
	return nil
}

// Gateways returns nothing on Android. Routing is owned by VpnService.
func Gateways() []net.IP {
	return nil
}
