//go:build android

package atun

import (
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/safing/portmaster/base/log"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

const (
	// tcpReceiveWindow 0 lets gVisor use its default receive window.
	tcpReceiveWindow = 0

	// tcpMaxInFlight caps handshakes waiting for a verdict. Past it gVisor
	// ignores new SYNs and the app retransmits.
	tcpMaxInFlight = 2 << 10

	// Keepalive on the app-side endpoint finds apps that went away without a
	// FIN, so their upstream socket is released instead of idling forever.
	// The upstream dialer uses the same values to find dead peers. Each probe
	// wakes the engine, and upstream also the radio, so they are rare: a leak
	// guard, not a liveness check.
	tcpKeepaliveCount    = 9
	tcpKeepaliveIdle     = 10 * time.Minute
	tcpKeepaliveInterval = 75 * time.Second
)

func newTCPForwarder(s *stackSession) *tcp.Forwarder {
	return tcp.NewForwarder(s.stack, tcpReceiveWindow, tcpMaxInFlight, func(r *tcp.ForwarderRequest) {
		// gVisor runs this in its own goroutine, which Close cannot wait
		// for. The worker is tracked, so Close waits for the flow.
		if !s.runWorker("atun TCP flow", func() { handleTCPFlow(s, r) }) {
			r.Complete(true)
		}
	})
}

func handleTCPFlow(s *stackSession, r *tcp.ForwarderRequest) {
	id := r.ID()
	info := newPacketInfo(id, pmpacket.TCP)

	q := s.queueFor(info.Version == pmpacket.IPv6)
	if q == nil {
		// Family not intercepted.
		r.Complete(true)

		return
	}

	connID := pmpacket.CreateConnectionID(
		info.Protocol, info.Src, info.SrcPort, info.Dst, info.DstPort, false,
	)

	f, ok := flows.begin(connID, info)
	if !ok {
		// Dup SYN for a tuple with a verdict in flight. First attempt wins.
		// Drop this one silent. gVisor needs exactly one Complete per request.
		r.Complete(false)

		return
	}

	handedOff := false
	defer func() {
		if !handedOff {
			f.close()
		}
	}()

	mark := decide(s, q, f)
	// TCP DNS uses a 2-byte length prefix. The nameserver speaks UDP, so this
	// path translates instead of proxying the bytes through.
	dns := shouldHandleDNS(s, f.info, mark)

	switch {
	case dns, markIsAccept(mark), mark == MarkRerouteSPN, mark == MarkRerouteSplitTun, mark == MarkRerouteWireGuard:
		// Handled below.
	case markIsBlock(mark):
		// Complete(true) answers the SYN with RST, so the app fails fast.
		r.Complete(true)

		return
	default:
		// Drop: no RST, so the app sees a timeout, as with nfq's drop.
		r.Complete(false)

		return
	}

	var wq waiter.Queue
	ep, tcpErr := r.CreateEndpoint(&wq)
	if tcpErr != nil {
		// Always complete. Else slots fill and TCP stalls.
		r.Complete(true)

		return
	}

	setTCPConnOptions(s, ep)
	// Complete now: the in-flight slot is held until Complete, and this
	// connection may stay open for hours.
	r.Complete(false)

	appConn := gonet.NewTCPConn(&wq, ep)

	if dns {
		handleTCPDNSFlow(s, f, appConn)

		return
	}

	if mark == MarkRerouteSPN {
		if handOffToSPN(f, appConn) {
			handedOff = true

			return
		}
		// The user routed this through the SPN. Going direct would leak it.
		_ = appConn.Close()

		return
	}

	if mark == MarkRerouteWireGuard {
		proxyViaWireGuard(s.ctx, s, f, appConn, 0)

		return
	}

	if mark == MarkRerouteSplitTun {
		// Split tunnel means "leave outside the VPN", which is exactly what a
		// protected direct dial does.
		log.Tracef("atun: split-tunnelling %s directly", connID)
	}

	proxyFlow(s.ctx, s, f, appConn, 0)
}

// setTCPConnOptions applies keepalive and the stack's default buffer sizes to
// an accepted endpoint. Failures only cost tuning, so they are traced.
func setTCPConnOptions(s *stackSession, ep tcpip.Endpoint) {
	if s == nil || s.stack == nil || ep == nil {
		return
	}

	ep.SocketOptions().SetKeepAlive(true)

	idle := tcpip.KeepaliveIdleOption(tcpKeepaliveIdle)
	if err := ep.SetSockOpt(&idle); err != nil {
		log.Tracef("atun: tcp keepalive idle: %s", err)
	}

	interval := tcpip.KeepaliveIntervalOption(tcpKeepaliveInterval)
	if err := ep.SetSockOpt(&interval); err != nil {
		log.Tracef("atun: tcp keepalive interval: %s", err)
	}

	if err := ep.SetSockOptInt(tcpip.KeepaliveCountOption, tcpKeepaliveCount); err != nil {
		log.Tracef("atun: tcp keepalive count: %s", err)
	}

	var ss tcpip.TCPSendBufferSizeRangeOption
	if err := s.stack.TransportProtocolOption(header.TCPProtocolNumber, &ss); err == nil {
		ep.SocketOptions().SetSendBufferSize(int64(ss.Default), false)
	}

	var rs tcpip.TCPReceiveBufferSizeRangeOption
	if err := s.stack.TransportProtocolOption(header.TCPProtocolNumber, &rs); err == nil {
		ep.SocketOptions().SetReceiveBufferSize(int64(rs.Default), false)
	}
}
