//go:build android

package atun

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
	"github.com/safing/portmaster/service/nameserver/nsutil"
	pmpacket "github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/wireguard"
)

const (
	dialTimeout = 30 * time.Second

	// UDP has no FIN. Without this each exchange leaks a goroutine.
	udpIdleTimeout = 5 * time.Minute

	// After one side FINs, wait for the other direction to drain before tearing down.
	tcpWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize is the copy buffer per TCP direction. TCP is a
	// stream, so it need not fit a whole datagram.
	tcpRelayBufferSize = 20 << 10

	// udpMaxSegmentSize fits the largest UDP payload, so a datagram is never
	// split across two reads.
	udpMaxSegmentSize = (1 << 16) - 1
)

// Same key as nameserver. Copied to avoid an import cycle.
const nameserverAddressKey = "dns/listenAddress"

// The Android listen address uses port 0, which is not a dial target, so
// there is no usable default before the nameserver is bound.
const defaultNameserverAddress = ""

// dialer protects every upstream socket with VpnService.protect. Do not dial
// upstream with anything else: an unprotected socket is routed back into this
// TUN and loops.
//
// Keep-alive is slow, like the app-side endpoint's. A zero Dialer probes every
// 15 seconds once idle, so each idle app connection woke the radio that often.
// It cannot be off: an app's TCP keep-alive probes are answered by the stack
// and never reach this socket, so this probe is the only one that refreshes
// the NAT mapping of an idle connection and finds a dead upstream.
var dialer = aprotect.ApplyToDialer(&net.Dialer{
	KeepAliveConfig: net.KeepAliveConfig{
		Enable:   true,
		Idle:     tcpKeepaliveIdle,
		Interval: tcpKeepaliveInterval,
		Count:    tcpKeepaliveCount,
	},
})

var (
	tcpBufPool = sync.Pool{New: func() any {
		b := make([]byte, tcpRelayBufferSize)
		return &b
	}}
	udpBufPool = sync.Pool{New: func() any {
		b := make([]byte, udpMaxSegmentSize)
		return &b
	}}
)

func portToString(port uint16) string {
	return strconv.Itoa(int(port))
}

func nameserverAddress() string {
	if addr := nsutil.BoundAddress(); addr != "" {
		return addr
	}
	addr := config.GetAsString(nameserverAddressKey, defaultNameserverAddress)()
	if addr == "" || strings.HasSuffix(addr, ":0") {
		return ""
	}
	return addr
}

// networkForAddress picks the dial network for the flow.
// The address decides the family, as it need not share the flow's: the
// loopback nameserver answers v6 flows over v4.
func networkForAddress(info pmpacket.Info, address string) string {
	proto := "tcp"
	if info.Protocol == pmpacket.UDP {
		proto = "udp"
	}

	v6 := info.Version == pmpacket.IPv6
	if host, _, err := net.SplitHostPort(address); err == nil {
		if parsed := net.ParseIP(host); parsed != nil {
			v6 = parsed.To4() == nil
		}
	}
	if v6 {
		return proto + "6"
	}

	return proto + "4"
}

func dialUpstream(ctx context.Context, info pmpacket.Info) (net.Conn, error) {
	if !aprotect.HasProtector() {
		return nil, ErrNoProtector
	}

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	address := dstAddr(info)
	network := networkForAddress(info, address)

	conn, err := dialer.DialContext(dialCtx, network, address)
	if err != nil {
		return nil, fmt.Errorf("atun: dial %s %s: %w", network, address, err)
	}

	return conn, nil
}

// dialNameserver dials the local resolver for a DNS flow.
// A UDP dial does not fail on an absent listener. A missing nameserver shows
// up as ECONNREFUSED on the first read, which ends the flow.
func dialNameserver(ctx context.Context, info pmpacket.Info) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	// The resolver is UDP only, whatever the app used to reach us.
	info.Protocol = pmpacket.UDP

	address := nameserverAddress()
	network := networkForAddress(info, address)

	conn, err := dialer.DialContext(dialCtx, network, address)
	if err != nil {
		return nil, fmt.Errorf("atun: dial nameserver %s %s: %w", network, address, err)
	}

	return conn, nil
}

// dialWireGuard dials the flow's destination through the WireGuard tunnel.
// It fails closed: the tunnel does not fall back to a direct socket, because
// that is the leak the routing target exists to prevent.
func dialWireGuard(ctx context.Context, info pmpacket.Info) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	address := dstAddr(info)
	network := networkForAddress(info, address)

	conn, err := wireguard.DialContext(dialCtx, network, address)
	if err != nil {
		return nil, fmt.Errorf("atun: wireguard dial %s %s: %w", network, address, err)
	}

	return conn, nil
}

type (
	closeReader interface{ CloseRead() error }
	closeWriter interface{ CloseWrite() error }
)

// proxy copies both ways until both directions finish, then closes the flow.
// UDP: idle timeout ends session. TCP: CloseWrite so client FIN doesn't kill server response.
func proxy(s *stackSession, f *flow, appConn, upstream net.Conn, idleTimeout time.Duration) {
	defer f.close()

	if idleTimeout > 0 {
		proxyPackets(s, appConn, upstream, idleTimeout)
		return
	}
	proxyStream(s, appConn, upstream)
}

func proxyStream(s *stackSession, appConn, upstream net.Conn) {
	var wg sync.WaitGroup
	var halfClosed atomic.Bool
	wg.Add(2)
	start := func(name string, dst, src net.Conn) {
		if !s.runWorker(name, func() {
			defer wg.Done()
			pipeTCP(dst, src, &halfClosed)
		}) {
			wg.Done()
		}
	}
	start("atun proxy app-to-upstream", upstream, appConn)
	start("atun proxy upstream-to-app", appConn, upstream)
	wg.Wait()
}

// pipeTCP copies TCP data with proper half-close handling. Once either
// direction finished, the other one ends after tcpWaitTimeout without data.
func pipeTCP(dst, src net.Conn, halfClosed *atomic.Bool) {
	bufp, ok := tcpBufPool.Get().(*[]byte)
	if !ok {
		return
	}
	_, _ = io.CopyBuffer(dst, idleAfterHalfClose{conn: src, halfClosed: halfClosed}, *bufp)
	tcpBufPool.Put(bufp)

	if cr, ok := src.(closeReader); ok {
		_ = cr.CloseRead()
	}
	if cw, ok := dst.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
	halfClosed.Store(true)
	_ = dst.SetReadDeadline(time.Now().Add(tcpWaitTimeout))
}

// idleAfterHalfClose extends the read deadline before each read once the
// connection is half closed, so the deadline set at the half close does
// not cut off a transfer that is still running.
type idleAfterHalfClose struct {
	conn       net.Conn
	halfClosed *atomic.Bool
}

func (r idleAfterHalfClose) Read(p []byte) (int, error) {
	if r.halfClosed.Load() {
		_ = r.conn.SetReadDeadline(time.Now().Add(tcpWaitTimeout))
	}
	return r.conn.Read(p)
}

func proxyPackets(s *stackSession, appConn, upstream net.Conn, idleTimeout time.Duration) {
	var wg sync.WaitGroup
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = appConn.Close()
			_ = upstream.Close()
		})
	}

	wg.Add(2)
	start := func(name string, dst, src net.Conn) {
		if !s.runWorker(name, func() {
			defer wg.Done()
			copyDatagrams(dst, src, idleTimeout)
			closeBoth()
		}) {
			wg.Done()
			closeBoth()
		}
	}
	start("atun proxy app-to-upstream", upstream, appConn)
	start("atun proxy upstream-to-app", appConn, upstream)
	wg.Wait()
}

// copyDatagrams copies UDP datagrams; 64KiB buffer preserves message boundaries.
func copyDatagrams(dst, src net.Conn, idleTimeout time.Duration) {
	bufp, ok := udpBufPool.Get().(*[]byte)
	if !ok {
		return
	}
	defer udpBufPool.Put(bufp)
	buf := *bufp

	for {
		if idleTimeout > 0 {
			_ = src.SetReadDeadline(time.Now().Add(idleTimeout))
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return
			}
			if idleTimeout > 0 {
				_ = dst.SetReadDeadline(time.Now().Add(idleTimeout))
			}
		}
		if readErr != nil {
			return
		}
	}
}

func proxyViaWireGuard(ctx context.Context, s *stackSession, f *flow, appConn net.Conn, idleTimeout time.Duration) {
	f.addCloser(appConn)

	upstream, err := dialWireGuard(ctx, f.info)
	if err != nil {
		log.Tracef("atun: %s", err)
		f.close()

		return
	}
	f.addCloser(upstream)

	// Dial raced with teardown. Late conns close at once.
	if f.isClosed() {
		return
	}

	proxy(s, f, &countingConn{Conn: appConn, flow: f}, upstream, idleTimeout)
}

func proxyFlow(ctx context.Context, s *stackSession, f *flow, appConn net.Conn, idleTimeout time.Duration) {
	f.addCloser(appConn)

	upstream, err := dialUpstream(ctx, f.info)
	if err != nil {
		log.Tracef("atun: %s", err)
		f.close()

		return
	}
	f.addCloser(upstream)

	// Dial raced with teardown. Late conns close at once.
	if f.isClosed() {
		return
	}

	proxy(s, f, &countingConn{Conn: appConn, flow: f}, upstream, idleTimeout)
}
