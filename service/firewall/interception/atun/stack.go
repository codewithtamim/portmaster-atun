//go:build android

package atun

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"

	"github.com/safing/portmaster/service/mgr"
)

const defaultMTU = 1500

// defaultTunAddr is the IPv4 VpnService.Builder.addAddress value.
// gVisor must hold it or locally destined packets (ICMP, DNS replies) have
// no source and never leave the TUN.
const defaultTunAddr = "100.127.247.245"

// defaultTunAddr6 is the IPv6 VpnService.Builder.addAddress value.
// The same TUN carries IPv6 so apps cannot bypass the firewall on v6.
const defaultTunAddr6 = "fd00:9:9:9::1"

// defaultFakeDNSAddr is the IPv4 handed to Builder.addDnsServer.
// Nothing listens there. atun answers for it and forwards to the nameserver.
const defaultFakeDNSAddr = "10.111.222.3"

const defaultHopLimit = 64

// ICMP reply budget. Without this a ping flood from the TUN becomes a
// reply flood we generate ourselves.
const (
	icmpBurst = 50
	icmpLimit = rate.Limit(1000)
)

// Android contract (libpm-go StartTun and the JNI callbacks):
//
//	SetTunConfig                      the borrowed TUN fd, MTU, TUN and fake DNS IPv4
//	aprotect.RegisterProtector        VpnService.protect(fd)
//	aconn.RegisterOwnerResolver       ConnectivityManager.getConnectionOwnerUid
//	aconn.RegisterAppInfoProvider     PackageManager JSON
//
// Dup model: Java keeps its fd. Go owns only the unix.Dup. Kept as int;
// os.File would flip blocking mode. fdbased.Close is a no-op in this gVisor,
// so we close the dup after stack.Wait(). Java closes its PFD separately.

// stackSession is one VPN session. The package session holds it because one
// TUN fd serves both IP-family queues.
type stackSession struct {
	tunFD  int
	device stack.LinkEndpoint
	stack  *stack.Stack
	nicID  tcpip.NICID

	// tunAddr is the address assigned to the gVisor NIC. Must match
	// VpnService.Builder.addAddress or replies never leave the TUN.
	tunAddr netip.Addr

	// fakeDNS is the address handed to apps via addDnsServer.
	// Queries to it always go to our resolver, never upstream.
	fakeDNS netip.Addr

	ctx      context.Context
	cancel   context.CancelFunc
	mgr      *mgr.Manager
	ownedMgr bool
	closed   atomic.Bool

	// Split by family like nfqueue. Nil means not intercepted.
	queue4 *Queue
	queue6 *Queue

	// mu also orders runWorker against Close, so no worker is added after
	// Close started waiting for them.
	mu      sync.Mutex
	workers sync.WaitGroup
}

func (s *stackSession) queueFor(v6 bool) *Queue {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v6 {
		return s.queue6
	}

	return s.queue4
}

func (s *stackSession) runWorker(name string, fn func()) bool {
	if s == nil || s.mgr == nil {
		return false
	}
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		return false
	}
	s.workers.Add(1)
	s.mu.Unlock()

	s.mgr.Go(name, func(_ *mgr.WorkerCtx) error {
		defer s.workers.Done()
		fn()
		return nil
	})
	return true
}

// dupTunFD dups fd for Go. Caller owns the copy.
func dupTunFD(fd int) (int, error) {
	if fd < 0 {
		return -1, ErrInvalidFD
	}
	newfd, err := unix.Dup(fd)
	if err != nil {
		return -1, fmt.Errorf("atun: dup TUN fd: %w", err)
	}
	if err := unix.SetNonblock(newfd, true); err != nil {
		_ = unix.Close(newfd)
		return -1, fmt.Errorf("atun: set TUN fd nonblock: %w", err)
	}
	return newfd, nil
}

func addTunProtocolAddress(s *stack.Stack, nicID tcpip.NICID, addr netip.Addr) error {
	var protocolAddr tcpip.ProtocolAddress
	switch {
	case addr.Is4():
		protocolAddr = tcpip.ProtocolAddress{
			Protocol:          ipv4.ProtocolNumber,
			AddressWithPrefix: tcpip.AddrFrom4(addr.As4()).WithPrefix(),
		}
	case addr.Is6():
		protocolAddr = tcpip.ProtocolAddress{
			Protocol:          ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddrFrom16(addr.As16()).WithPrefix(),
		}
	default:
		return fmt.Errorf("atun: unsupported address family: %s", addr)
	}
	if err := s.AddProtocolAddress(nicID, protocolAddr, stack.AddressProperties{}); err != nil {
		return fmt.Errorf("atun: add TUN address %s: %s", addr, err)
	}
	return nil
}

// newStack builds a session on cfg. cfg comes from NewTunConfig, so its
// addresses are already parsed and the DNS hot path never parses.
func newStack(cfg TunConfig, workerManager *mgr.Manager) (*stackSession, error) {
	ownedMgr := workerManager == nil
	if ownedMgr {
		workerManager = mgr.New("ATUN")
	}
	fakeDNS, tunAddr, mtu := cfg.FakeDNS, cfg.Addr, cfg.MTU
	tunFD, err := dupTunFD(cfg.FD)
	if err != nil {
		return nil, err
	}

	// TUN holds raw IP. No Ethernet header.
	ep, err := fdbased.New(&fdbased.Options{
		FDs:            []int{tunFD},
		MTU:            mtu,
		EthernetHeader: false,
	})
	if err != nil {
		_ = unix.Close(tunFD)
		return nil, fmt.Errorf("atun: create fdbased endpoint: %w", err)
	}

	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
			icmp.NewProtocol6,
		},
	})

	// Forwarders need the session, so build it first.
	ctx, cancel := context.WithCancel(context.Background())
	st := &stackSession{
		tunFD:    tunFD,
		device:   ep,
		stack:    s,
		tunAddr:  tunAddr,
		fakeDNS:  fakeDNS,
		ctx:      ctx,
		cancel:   cancel,
		mgr:      workerManager,
		ownedMgr: ownedMgr,
	}

	tcpFwd := newTCPForwarder(st)
	udpFwd := newUDPForwarder(st)

	// Handlers before CreateNIC. Dispatch starts the moment the NIC exists,
	// so a packet that arrives first would be dropped. ICMP is ours because
	// gVisor skips Echo Reply for promiscuous (temporary) addresses;
	// see icmp_forwarder.go.
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd)
	s.SetTransportProtocolHandler(icmp.ProtocolNumber4, newICMPv4Handler(st))
	s.SetTransportProtocolHandler(icmp.ProtocolNumber6, newICMPv6Handler(st))

	// Same unwind as Close. fdbased does not close the fd.
	abort := func() {
		cancel()
		s.Close()
		s.Wait()
		_ = unix.Close(tunFD)
		st.tunFD = -1
	}

	// NIC last. CreateNIC starts dispatch at once.
	nicID := s.NextNICID()
	if err := s.CreateNICWithOptions(nicID, ep, stack.NICOptions{
		Disabled: false,
		QDisc:    nil,
	}); err != nil {
		abort()
		return nil, fmt.Errorf("atun: create NIC: %s", err)
	}
	if err := addTunProtocolAddress(s, nicID, tunAddr); err != nil {
		abort()
		return nil, err
	}
	tunAddr6, err := netip.ParseAddr(defaultTunAddr6)
	if err != nil {
		abort()
		return nil, fmt.Errorf("atun: parse TUN IPv6: %w", err)
	}
	if err := addTunProtocolAddress(s, nicID, tunAddr6); err != nil {
		abort()
		return nil, err
	}
	// Promiscuous accepts packets for any destination, spoofing lets replies
	// leave from that address. Every terminated flow needs both.
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		abort()
		return nil, fmt.Errorf("atun: set promiscuous: %s", err)
	}
	if err := s.SetSpoofing(nicID, true); err != nil {
		abort()
		return nil, fmt.Errorf("atun: set spoofing: %s", err)
	}
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: nicID},
		{Destination: header.IPv6EmptySubnet, NIC: nicID},
	})

	if err := configureStack(s); err != nil {
		abort()
		return nil, err
	}

	st.nicID = nicID

	return st, nil
}

// configureStack tunes the stack for single-NIC termination: no forwarding
// between NICs, and no multicast dummy addresses (Android mDNS never
// enters the TUN).
func configureStack(s *stack.Stack) error {
	defaultTTL := tcpip.DefaultTTLOption(defaultHopLimit)
	if err := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &defaultTTL); err != nil {
		return fmt.Errorf("atun: set ipv4 ttl: %s", err)
	}
	if err := s.SetNetworkProtocolOption(ipv6.ProtocolNumber, &defaultTTL); err != nil {
		return fmt.Errorf("atun: set ipv6 ttl: %s", err)
	}

	s.SetICMPBurst(icmpBurst)
	s.SetICMPLimit(icmpLimit)

	sendBuf := tcpip.TCPSendBufferSizeRangeOption{
		Min:     tcp.MinBufferSize,
		Default: tcp.DefaultSendBufferSize,
		Max:     tcp.MaxBufferSize,
	}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sendBuf); err != nil {
		return fmt.Errorf("atun: set tcp send buffer: %s", err)
	}
	recvBuf := tcpip.TCPReceiveBufferSizeRangeOption{
		Min:     tcp.MinBufferSize,
		Default: tcp.DefaultReceiveBufferSize,
		Max:     tcp.MaxBufferSize,
	}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &recvBuf); err != nil {
		return fmt.Errorf("atun: set tcp receive buffer: %s", err)
	}

	cc := tcpip.CongestionControlOption("reno")
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &cc); err != nil {
		return fmt.Errorf("atun: set tcp congestion control: %s", err)
	}
	delayEnabled := tcpip.TCPDelayEnabled(false)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &delayEnabled); err != nil {
		return fmt.Errorf("atun: set tcp delay: %s", err)
	}
	moderateRecv := tcpip.TCPModerateReceiveBufferOption(false)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &moderateRecv); err != nil {
		return fmt.Errorf("atun: set tcp moderate receive buffer: %s", err)
	}
	sackEnabled := tcpip.TCPSACKEnabled(true)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sackEnabled); err != nil {
		return fmt.Errorf("atun: set tcp sack: %s", err)
	}
	recovery := tcpip.TCPRACKLossDetection
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &recovery); err != nil {
		return fmt.Errorf("atun: set tcp recovery: %s", err)
	}

	return nil
}

// Close stops one VPN session: cancel handlers, drop flows, stack.Close/Wait,
// then close Go's dup of the TUN fd. fdbased.Close is a no-op, so this is the
// only close of that integer; Java closes its own ParcelFileDescriptor separately.
// Do not close the dup before stack.Wait — dispatchers still read from it.
func (s *stackSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		return nil
	}
	s.closed.Store(true)
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	TeardownFlowTable()
	if s.stack != nil {
		s.stack.Close()
		s.stack.Wait()
	}
	s.workers.Wait()
	if s.ownedMgr && s.mgr != nil {
		s.mgr.Cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tunFD >= 0 {
		_ = unix.Close(s.tunFD)
		s.tunFD = -1
	}
	// Leave packet channels open. nfq.Destroy does the same. Closing them
	// would make handleInterception receive a nil packet before shutdownSignal.
	return nil
}

func (s *stackSession) Wait() {
	if s == nil || s.stack == nil {
		return
	}
	s.stack.Wait()
}
