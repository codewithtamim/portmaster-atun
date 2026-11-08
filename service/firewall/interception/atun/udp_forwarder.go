//go:build android

package atun

import (
	"errors"
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

	connID := pmpacket.CreateConnectionID(
		info.Protocol, info.Src, info.SrcPort, info.Dst, info.DstPort, false,
	)

	f, err := flows.begin(connID, info)
	if err != nil {
		if errors.Is(err, errFlowLimit) {
			flowLimitLog.warn("atun: udp flow limit reached, dropping %s", dstAddr(info))
		}
		// Else the tuple already has a live flow. Keep that one.
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

	v := decide(s, q, f)
	mark := v.mark

	// Fake DNS is a black hole unless we answer it. Honour that before Block
	// so a nameserver that is still starting does not drop system DNS. With
	// no verdict at all the app is unknown, so answer SERVFAIL instead of
	// relaying unchecked.
	if shouldHandleDNS(s, f.info, mark) {
		if !v.decided {
			log.Warningf("atun: no verdict for dns flow %s, answering SERVFAIL", connID)
			replyServFail(appConn)

			return
		}
		handleDNSFlow(s, f, appConn)

		return
	}

	switch {
	case markIsAccept(mark), mark == MarkRerouteSPN, mark == MarkRerouteSplitTun, mark == MarkRerouteWireGuard:
		// Handled below.
	case markIsBlock(mark):
		log.Tracef("atun: blocking udp %s", connID)
		// Fail fast, like the RST a blocked TCP flow gets.
		rejectUDP(s, id)

		return
	default:
		return
	}

	if !flows.charge(f, v.uid) {
		flowLimitLog.warn("atun: udp flow limit reached for uid %s, dropping %s", uidString(v.uid), dstAddr(f.info))

		return
	}

	if mark == MarkRerouteSPN {
		if handOffToSPN(f, appConn, udpIdleTimeout) {
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
