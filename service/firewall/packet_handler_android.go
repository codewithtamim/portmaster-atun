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
