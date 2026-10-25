//go:build android

package atun

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/miekg/dns"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/network"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

const (
	dnsPort = 53

	// DNS is one exchange, not a session. proxy.go's five minutes would pin a
	// socket and a flow entry per query.
	dnsIdleTimeout = 30 * time.Second

	dnsReplyTimeout = time.Second

	// dnsLingerTimeout is how long a UDP DNS flow waits for another query
	// after a reply. c-ares sends the A and AAAA queries on one socket.
	dnsLingerTimeout = time.Second
)

func isDNSPort(info pmpacket.Info) bool {
	return info.DstPort == dnsPort
}

// isFakeDNSTarget checks against our advertised DNS server address (unmapping v4-mapped-v6).
func isFakeDNSTarget(s *stackSession, info pmpacket.Info) bool {
	if s == nil || !isDNSPort(info) || !s.fakeDNS.IsValid() {
		return false
	}
	dst, ok := netip.AddrFromSlice(info.Dst)
	if !ok {
		return false
	}

	return dst.Unmap() == s.fakeDNS.Unmap()
}

// shouldHandleDNS is true for firewall-rerouted DNS or queries to the fake DNS
// server (dstIP == fakeDNS && dstPort == 53). Do not match on port 53 alone —
// that would also catch apps' own resolvers and loop our upstream queries back.
func shouldHandleDNS(s *stackSession, info pmpacket.Info, mark int) bool {
	if !isDNSPort(info) {
		return false
	}

	return isFakeDNSTarget(s, info) || mark == MarkRerouteNS
}

// dialDNSUpstream dials the nameserver for f. On a dial failure it logs, sends
// a SERVFAIL reply through replyFail, closes f and returns (nil, false). On
// success it registers the closers and DNS attribution and returns
// (upstream, true); if the flow was already closed during dial it returns
// (upstream, false) without attribution.
func dialDNSUpstream(s *stackSession, f *flow, appConn net.Conn, replyFail func(net.Conn)) (net.Conn, bool) {
	f.addCloser(appConn)

	upstream, err := dialNameserver(s.ctx, f.info)
	if err != nil {
		log.Warningf("atun: %s", err)
		replyFail(appConn)
		f.close()

		return nil, false
	}
	f.addCloser(upstream)

	// Dial raced with teardown. Late conns close at once.
	if f.isClosed() {
		return upstream, false
	}

	// Register so resolver sees query from the app, not from us.
	registerDNSAttribution(s, f, upstream)

	return upstream, true
}

// handleDNSFlow forwards DNS to the resolver and closes when done.
func handleDNSFlow(s *stackSession, f *flow, appConn net.Conn) {
	upstream, ok := dialDNSUpstream(s, f, appConn, replyServFail)
	if !ok {
		return
	}

	counted := &countingConn{Conn: appConn, flow: f}
	if f.info.Protocol == pmpacket.UDP {
		if err := relayUDPDNS(counted, upstream); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			log.Tracef("atun: udp dns %s: %s", dstAddr(f.info), err)
		}
		f.close()
		return
	}

	proxy(s, f, counted, upstream, dnsIdleTimeout)
}

// relayUDPDNS forwards the queries of one UDP DNS socket one at a time. After
// a reply it waits only dnsLingerTimeout for another query, instead of
// holding the flow for the idle timeout.
func relayUDPDNS(appConn, upstream net.Conn) error {
	bufp, ok := udpBufPool.Get().(*[]byte)
	if !ok {
		return nil
	}
	defer udpBufPool.Put(bufp)
	buf := *bufp

	waitForQuery := dnsIdleTimeout
	for {
		_ = appConn.SetReadDeadline(time.Now().Add(waitForQuery))
		n, err := appConn.Read(buf)
		if err != nil || n == 0 {
			if waitForQuery == dnsLingerTimeout {
				// A query was answered; the app sent no other one.
				return nil
			}
			return err
		}
		_ = upstream.SetDeadline(time.Now().Add(dnsIdleTimeout))
		if _, err = upstream.Write(buf[:n]); err != nil {
			return err
		}
		n, err = upstream.Read(buf)
		if n <= 0 || err != nil {
			return err
		}
		if _, err = appConn.Write(buf[:n]); err != nil {
			return err
		}
		waitForQuery = dnsLingerTimeout
	}
}

// handleTCPDNSFlow answers a TCP DNS session. Each length-prefixed query is
// one UDP datagram to the nameserver, and the reply is written back with the
// same prefix. The nameserver itself stays UDP.
func handleTCPDNSFlow(s *stackSession, f *flow, appConn net.Conn) {
	upstream, ok := dialDNSUpstream(s, f, appConn, replyTCPServFail)
	if !ok {
		return
	}

	log.Tracef("atun: tcp dns %s", dstAddr(f.info))

	if err := relayTCPDNS(&countingConn{Conn: appConn, flow: f}, upstream); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		log.Tracef("atun: tcp dns %s: %s", dstAddr(f.info), err)
	}
	f.close()
}

// relayTCPDNS copies one TCP DNS stream onto a connected UDP nameserver socket.
func relayTCPDNS(appConn, upstream net.Conn) error {
	buf := make([]byte, udpMaxSegmentSize)
	for {
		_ = appConn.SetReadDeadline(time.Now().Add(dnsIdleTimeout))
		query, err := readTCPDNS(appConn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}

		_ = upstream.SetDeadline(time.Now().Add(dnsIdleTimeout))
		if _, err = upstream.Write(query); err != nil {
			return err
		}
		n, err := upstream.Read(buf)
		if n <= 0 || err != nil {
			return err
		}
		_ = appConn.SetWriteDeadline(time.Now().Add(dnsReplyTimeout))
		if err = writeTCPDNS(appConn, buf[:n]); err != nil {
			return err
		}
	}
}

func readTCPDNS(conn net.Conn) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func writeTCPDNS(conn net.Conn, msg []byte) error {
	if len(msg) == 0 || len(msg) > 0xFFFF {
		return io.ErrShortBuffer
	}
	var frame [2]byte
	binary.BigEndian.PutUint16(frame[:], uint16(len(msg)))
	if _, err := conn.Write(frame[:]); err != nil {
		return err
	}
	_, err := conn.Write(msg)

	return err
}

// registerDNSAttribution maps flow identity to the loopback socket for app attribution.
func registerDNSAttribution(s *stackSession, f *flow, upstream net.Conn) {
	local, ok := upstream.LocalAddr().(*net.UDPAddr)
	if !ok || local == nil {
		log.Warningf("atun: no local UDP address for dns flow %s, request will be misattributed", f.id)

		return
	}

	conn, ok := network.GetConnection(f.id)
	if !ok {
		log.Warningf("atun: no connection found for dns flow %s, request will be misattributed", f.id)

		return
	}

	version := pmpacket.IPv6
	if local.IP.To4() != nil {
		version = pmpacket.IPv4
	}

	// Src only. The nameserver does not record direction.
	pkt := pmpacket.NewInfoPacket(pmpacket.Info{
		Inbound:  false,
		Version:  version,
		Protocol: pmpacket.UDP,
		Src:      local.IP,
		SrcPort:  uint16(local.Port),
		PID:      conn.PID,
		UID:      conn.UID,
		SeenAt:   time.Now(),
	})
	pkt.SetCtx(s.ctx)

	network.SaveDNSRequestConnection(conn, pkt)
}

// servFailReply packs a SERVFAIL response for the given query bytes. It returns
// nil when the query cannot be unpacked or the reply cannot be packed, so the
// caller simply drops the flow.
func servFailReply(raw []byte) []byte {
	var query dns.Msg
	if unpackErr := query.Unpack(raw); unpackErr != nil {
		return nil
	}

	var reply dns.Msg
	reply.SetRcode(&query, dns.RcodeServerFailure)
	packed, packErr := reply.Pack()
	if packErr != nil {
		return nil
	}

	return packed
}

// replyServFail sends SERVFAIL so apps fail fast instead of timing out.
func replyServFail(appConn net.Conn) {
	buf := make([]byte, udpMaxSegmentSize)

	_ = appConn.SetReadDeadline(time.Now().Add(dnsReplyTimeout))
	n, err := appConn.Read(buf)
	if n <= 0 || err != nil {
		return
	}

	packed := servFailReply(buf[:n])
	if packed == nil {
		return
	}

	_ = appConn.SetWriteDeadline(time.Now().Add(dnsReplyTimeout))
	_, _ = appConn.Write(packed)
}

// replyTCPServFail answers one length-prefixed query with SERVFAIL.
func replyTCPServFail(appConn net.Conn) {
	_ = appConn.SetReadDeadline(time.Now().Add(dnsReplyTimeout))
	raw, err := readTCPDNS(appConn)
	if err != nil {
		return
	}

	packed := servFailReply(raw)
	if packed == nil {
		return
	}

	_ = appConn.SetWriteDeadline(time.Now().Add(dnsReplyTimeout))
	_ = writeTCPDNS(appConn, packed)
}
