//go:build android

package interception

// RerouteToWireGuard passes the WireGuard reroute through to the traced
// packet, which on Android is an atun packet.
func (p *tracedPacket) RerouteToWireGuard() error {
	defer p.markServed("reroute-wireguard")
	if r, ok := p.Packet.(interface{ RerouteToWireGuard() error }); ok {
		return r.RerouteToWireGuard()
	}
	return p.Packet.Drop()
}
