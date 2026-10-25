//go:build !android

package firewall

import "github.com/safing/portmaster/service/network/packet"

func acceptEmptyDNSPayload(packet.Packet) bool {
	return false
}

// packetHasLayers is always true: nfqueue and the kernel extension always
// deliver parsed packets.
func packetHasLayers(packet.Packet) bool {
	return true
}
