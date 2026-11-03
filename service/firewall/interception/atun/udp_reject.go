//go:build android

package atun

import (
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/checksum"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// udpRejectQuoteTTL is the TTL written into the quoted datagram header. The
// receiver only reads the addresses, protocol and ports from it.
const udpRejectQuoteTTL = 64

// rejectUDP answers a blocked UDP flow with ICMP port unreachable, so the
// app's socket fails at once (ECONNREFUSED) instead of waiting for a reply
// that never comes. TCP gets the same effect from a RST.
//
// The verdict comes after the first datagram was consumed, so the quoted
// datagram is rebuilt from the flow's addresses and ports. The kernel matches
// an ICMP error to a socket by that 5-tuple only.
func rejectUDP(s *stackSession, id stack.TransportEndpointID) bool {
	if s == nil || s.stack == nil || s.closed.Load() {
		return false
	}
	// id.LocalAddress is the destination the app wrote to, id.RemoteAddress
	// the app itself. The error goes back from the destination to the app.
	if id.RemoteAddress.Len() == header.IPv4AddressSize {
		return rejectUDPv4(s, id)
	}
	return rejectUDPv6(s, id)
}

func quotedUDPHeader(id stack.TransportEndpointID) []byte {
	udp := header.UDP(make([]byte, header.UDPMinimumSize))
	udp.Encode(&header.UDPFields{
		SrcPort: id.RemotePort,
		DstPort: id.LocalPort,
		Length:  header.UDPMinimumSize,
	})
	return udp
}

func rejectUDPv4(s *stackSession, id stack.TransportEndpointID) bool {
	r, err := s.stack.FindRoute(s.nicID, id.LocalAddress, id.RemoteAddress, ipv4.ProtocolNumber, false /* multicastLoop */)
	if err != nil {
		return false
	}
	defer r.Release()

	// ICMP header, then the quoted IPv4 header and the first 8 bytes of the
	// datagram (RFC 792).
	quotedIP := header.IPv4(make([]byte, header.IPv4MinimumSize))
	quotedIP.Encode(&header.IPv4Fields{
		TotalLength: header.IPv4MinimumSize + header.UDPMinimumSize,
		TTL:         udpRejectQuoteTTL,
		Protocol:    uint8(header.UDPProtocolNumber),
		SrcAddr:     id.RemoteAddress,
		DstAddr:     id.LocalAddress,
	})
	quotedIP.SetChecksum(^quotedIP.CalculateChecksum())

	msg := make([]byte, header.ICMPv4MinimumSize, header.ICMPv4MinimumSize+header.IPv4MinimumSize+header.UDPMinimumSize)
	msg = append(msg, quotedIP...)
	msg = append(msg, quotedUDPHeader(id)...)
	icmp := header.ICMPv4(msg)
	icmp.SetType(header.ICMPv4DstUnreachable)
	icmp.SetCode(header.ICMPv4PortUnreachable)
	icmp.SetChecksum(0)
	icmp.SetChecksum(^checksum.Checksum(msg, 0))

	return writeReject(s, r, header.ICMPv4ProtocolNumber, msg)
}

func rejectUDPv6(s *stackSession, id stack.TransportEndpointID) bool {
	r, err := s.stack.FindRoute(s.nicID, id.LocalAddress, id.RemoteAddress, ipv6.ProtocolNumber, false /* multicastLoop */)
	if err != nil {
		return false
	}
	defer r.Release()

	quotedIP := header.IPv6(make([]byte, header.IPv6MinimumSize))
	quotedIP.Encode(&header.IPv6Fields{
		PayloadLength:     header.UDPMinimumSize,
		TransportProtocol: header.UDPProtocolNumber,
		HopLimit:          udpRejectQuoteTTL,
		SrcAddr:           id.RemoteAddress,
		DstAddr:           id.LocalAddress,
	})

	msg := make([]byte, header.ICMPv6DstUnreachableMinimumSize, header.ICMPv6DstUnreachableMinimumSize+header.IPv6MinimumSize+header.UDPMinimumSize)
	msg = append(msg, quotedIP...)
	msg = append(msg, quotedUDPHeader(id)...)
	icmp := header.ICMPv6(msg)
	icmp.SetType(header.ICMPv6DstUnreachable)
	icmp.SetCode(header.ICMPv6PortUnreachable)
	icmp.SetChecksum(0)
	icmp.SetChecksum(header.ICMPv6Checksum(header.ICMPv6ChecksumParams{
		Header: icmp,
		Src:    r.LocalAddress(),
		Dst:    r.RemoteAddress(),
	}))

	return writeReject(s, r, header.ICMPv6ProtocolNumber, msg)
}

func writeReject(s *stackSession, r *stack.Route, protocol tcpip.TransportProtocolNumber, msg []byte) bool {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		ReserveHeaderBytes: int(r.MaxHeaderLength()),
		Payload:            buffer.MakeWithData(msg),
	})
	defer pkt.DecRef()

	if err := r.WritePacket(stack.NetworkHeaderParams{
		Protocol: protocol,
		TTL:      r.DefaultTTL(),
	}, pkt); err != nil {
		return false
	}
	return true
}
