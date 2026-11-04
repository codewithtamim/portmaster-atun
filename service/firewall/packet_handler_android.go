//go:build android

package firewall

import "github.com/safing/portmaster/service/network/packet"

// acceptEmptyDNSPayload is true when the backend reported a DNS flow with no
// payload. The Android TUN integration does this. Inspection is best-effort,
// so the caller accepts instead of blocking what it cannot read.
func acceptEmptyDNSPayload(pkt packet.Packet) bool {
	return len(pkt.Payload()) == 0
}

// packetHasLayers reports whether pkt carries parsed headers. atun hands the
// firewall terminated flows, which have none.
func packetHasLayers(pkt packet.Packet) bool {
	return pkt.Layers() != nil
}

// wireGuardRerouter is a packet that can be handed to the WireGuard tunnel.
// Only the Android TUN backend provides it.
type wireGuardRerouter interface {
	RerouteToWireGuard() error
}

// rerouteToWireGuard hands the packet to the WireGuard tunnel, or drops it
// when the backend has no WireGuard target.
func rerouteToWireGuard(pkt packet.Packet) error {
	if r, ok := pkt.(wireGuardRerouter); ok {
		return r.RerouteToWireGuard()
	}
	return pkt.Drop()
}
