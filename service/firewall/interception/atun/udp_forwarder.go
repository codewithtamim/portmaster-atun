//go:build android

package atun

import (
	"net"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/safing/portmaster/base/log"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// newUDPForwarder creates the endpoint on the dispatcher goroutine, so later
// datagrams of the same 5-tuple queue on it while a worker waits for the
// verdict. It builds the request itself instead of using udp.NewForwarder:
// the request holds an extra PacketBuffer reference that udp.Forwarder never
// releases, so this owns a Clone and DecRefs it.
func newUDPForwarder(s *stackSession) func(stack.TransportEndpointID, *stack.PacketBuffer) bool {
	return func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
		owned := pkt.Clone()
		defer owned.DecRef()

		r := udp.NewForwarderRequest(s.stack, id, owned)
		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			// Endpoint-already-exists is common; return false for gVisor to handle.
			return false
		}

		appConn := gonet.NewUDPConn(&wq, ep)
		if !s.runWorker("atun UDP flow", func() {
			handleUDPFlow(s, id, appConn)
		}) {
			_ = appConn.Close()
		}

		return true
	}
}

func handleUDPFlow(s *stackSession, id stack.TransportEndpointID, appConn net.Conn) {
	info := newPacketInfo(id, pmpacket.UDP)

	q := s.queueFor(info.Version == pmpacket.IPv6)
	if q == nil {
		_ = appConn.Close()

		return
	}

	if flows.udpCount() >= maxUDPFlows {
		log.Warningf("atun: udp flow limit reached, dropping %s", dstAddr(info))
		_ = appConn.Close()
		return
	}

	connID := pmpacket.CreateConnectionID(
		info.Protocol, info.Src, info.SrcPort, info.Dst, info.DstPort, false,
	)

	f, ok := flows.begin(connID, info)
	if !ok {
		// The tuple already has a live flow. Keep that one.
		_ = appConn.Close()

		return
	}

	handedOff := false
	defer func() {
		if !handedOff {
			f.close()
		}
	}()

	f.addCloser(appConn)

	mark := decide(s, q, f)

	// Fake DNS is a black hole unless we answer it. Honour that before Block
	// so a nameserver that is still starting does not drop system DNS.
	if shouldHandleDNS(s, f.info, mark) {
		handleDNSFlow(s, f, appConn)

		return
	}

	switch {
	case markIsAccept(mark), mark == MarkRerouteSPN, mark == MarkRerouteSplitTun, mark == MarkRerouteWireGuard:
		// Handled below.
	case markIsBlock(mark):
		log.Tracef("atun: blocking udp %s", connID)

		return
	default:
		return
	}

	if mark == MarkRerouteSPN {
		if handOffToSPN(f, appConn) {
			handedOff = true

			return
		}
		_ = appConn.Close()

		return
	}

	if mark == MarkRerouteWireGuard {
		proxyViaWireGuard(s.ctx, s, f, appConn, udpIdleTimeout)

		return
	}

	if mark == MarkRerouteSplitTun {
		log.Tracef("atun: split-tunnelling %s directly", connID)
	}

	// UDP has no FIN; idle timeout ends the flow.
	proxyFlow(s.ctx, s, f, appConn, udpIdleTimeout)
}
