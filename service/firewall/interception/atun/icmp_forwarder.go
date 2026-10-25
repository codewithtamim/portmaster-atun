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

	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// Echo is terminated here. Android will not give us a raw ICMP socket, and
// SOCKS has no ICMP, so there is no way to ping the real destination.
// Block and drop stay silent so a blocked host does not look alive.
func newICMPv4Handler(s *stackSession) func(stack.TransportEndpointID, *stack.PacketBuffer) bool {
	return func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
		hdr := header.ICMPv4(pkt.TransportHeader().Slice())
		if len(hdr) < header.ICMPv4MinimumSize || hdr.Type() != header.ICMPv4Echo {
			// Let gVisor see the rest (unreachable, quench, …).
			return false
		}

		return handleICMPEcho(s, id, pkt, pmpacket.ICMP)
	}
}

func newICMPv6Handler(s *stackSession) func(stack.TransportEndpointID, *stack.PacketBuffer) bool {
	return func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
		hdr := header.ICMPv6(pkt.TransportHeader().Slice())
		if len(hdr) < header.ICMPv6MinimumSize || hdr.Type() != header.ICMPv6EchoRequest {
			return false
		}

		return handleICMPEcho(s, id, pkt, pmpacket.ICMPv6)
	}
}

func handleICMPEcho(s *stackSession, id stack.TransportEndpointID, pkt *stack.PacketBuffer, protocol pmpacket.IPProtocol) bool {
	if s == nil || s.mgr == nil {
		return true
	}

	// Clone so the worker can still read headers after gVisor reclaims pkt.
	owned := pkt.Clone()
	name := "atun ICMPv4 flow"
	if protocol == pmpacket.ICMPv6 {
		name = "atun ICMPv6 flow"
	}
	if !s.runWorker(name, func() {
		defer owned.DecRef()
		if !allowICMP(s, id, protocol) {
			return
		}
		if protocol == pmpacket.ICMPv6 {
			_ = replyICMPv6Echo(s, owned)
			return
		}
		_ = replyICMPv4Echo(s, owned)
	}) {
		owned.DecRef()
	}

	return true
}

func allowICMP(s *stackSession, id stack.TransportEndpointID, protocol pmpacket.IPProtocol) bool {
	q := s.queueFor(protocol == pmpacket.ICMPv6)
	if q == nil {
		return false
	}
	info := newPacketInfo(id, protocol)
	connID := pmpacket.CreateConnectionID(info.Protocol, info.Src, info.SrcPort, info.Dst, info.DstPort, false)
	f, ok := flows.begin(connID, info)
	if !ok {
		return false
	}
	defer f.close()

	return markIsAccept(decide(s, q, f))
}

// replyICMPv4Echo writes an Echo Reply back through the TUN.
func replyICMPv4Echo(s *stackSession, pkt *stack.PacketBuffer) bool {
	if s == nil || s.stack == nil || pkt == nil || s.closed.Load() {
		return false
	}

	ipHdr := header.IPv4(pkt.NetworkHeader().Slice())
	if len(ipHdr) < header.IPv4MinimumSize {
		return false
	}

	// RFC 1122 3.2.1.3: source must be one of our addresses, never
	// broadcast or multicast. Empty local lets spoofing pick the route.
	localAddr := ipHdr.DestinationAddress()
	if pkt.NetworkPacketInfo.LocalAddressBroadcast || header.IsV4MulticastAddress(localAddr) {
		localAddr = tcpip.Address{}
	}

	r, err := s.stack.FindRoute(pkt.NICID, localAddr, ipHdr.SourceAddress(), ipv4.ProtocolNumber, false /* multicastLoop */)
	if err != nil {
		return false
	}
	defer r.Release()

	replyData := stack.PayloadSince(pkt.TransportHeader())
	if replyData == nil {
		return false
	}
	defer replyData.Release()

	if !fillICMPv4EchoReply(replyData) {
		return false
	}

	replyPkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		ReserveHeaderBytes: int(r.MaxHeaderLength()),
		Payload:            buffer.MakeWithView(replyData.Clone()),
	})
	defer replyPkt.DecRef()

	sent := s.stack.Stats().ICMP.V4.PacketsSent
	if werr := r.WritePacket(stack.NetworkHeaderParams{
		Protocol: header.ICMPv4ProtocolNumber,
		TTL:      r.DefaultTTL(),
	}, replyPkt); werr != nil {
		sent.Dropped.Increment()

		return false
	}
	sent.EchoReply.Increment()

	return true
}

func fillICMPv4EchoReply(replyData *buffer.View) bool {
	if replyData == nil || replyData.Size() < header.ICMPv4MinimumSize {
		return false
	}

	replyICMPHdr := header.ICMPv4(replyData.AsSlice())
	replyICMPHdr.SetType(header.ICMPv4EchoReply)
	replyICMPHdr.SetCode(0) // RFC 792: Echo Reply has Code 0.
	replyICMPHdr.SetChecksum(0)
	replyICMPHdr.SetChecksum(^checksum.Checksum(replyData.AsSlice(), 0))

	return true
}

// replyICMPv6Echo is the v6 counterpart.
func replyICMPv6Echo(s *stackSession, pkt *stack.PacketBuffer) bool {
	if s == nil || s.stack == nil || pkt == nil || s.closed.Load() {
		return false
	}

	ipHdr := header.IPv6(pkt.NetworkHeader().Slice())
	if len(ipHdr) < header.IPv6MinimumSize {
		return false
	}

	srcAddr := ipHdr.SourceAddress()
	dstAddr := ipHdr.DestinationAddress()
	localAddr := dstAddr
	if header.IsV6MulticastAddress(dstAddr) {
		localAddr = tcpip.Address{}
	}

	r, err := s.stack.FindRoute(pkt.NICID, localAddr, srcAddr, ipv6.ProtocolNumber, false /* multicastLoop */)
	if err != nil {
		return false
	}
	defer r.Release()

	replyData := stack.PayloadSince(pkt.TransportHeader())
	if replyData == nil {
		return false
	}
	defer replyData.Release()

	if !fillICMPv6EchoReply(replyData, r.LocalAddress(), r.RemoteAddress()) {
		return false
	}

	replyTClass, _ := ipHdr.TOS()
	replyPkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		ReserveHeaderBytes: int(r.MaxHeaderLength()),
		Payload:            buffer.MakeWithView(replyData.Clone()),
	})
	defer replyPkt.DecRef()

	sent := s.stack.Stats().ICMP.V6.PacketsSent
	if werr := r.WritePacket(stack.NetworkHeaderParams{
		Protocol: header.ICMPv6ProtocolNumber,
		TTL:      r.DefaultTTL(),
		TOS:      replyTClass,
	}, replyPkt); werr != nil {
		sent.Dropped.Increment()

		return false
	}
	sent.EchoReply.Increment()

	return true
}

func fillICMPv6EchoReply(replyData *buffer.View, src, dst tcpip.Address) bool {
	if replyData == nil || replyData.Size() < header.ICMPv6EchoMinimumSize {
		return false
	}

	icmp := header.ICMPv6(replyData.AsSlice())
	icmp.SetType(header.ICMPv6EchoReply)
	icmp.SetChecksum(0)
	icmp.SetChecksum(header.ICMPv6Checksum(header.ICMPv6ChecksumParams{
		Header: icmp,
		Src:    src,
		Dst:    dst,
	}))

	return true
}
