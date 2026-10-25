//go:build android

package wireguard

import (
	"context"
	"net/netip"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/mgr"
)

// IPv6 reachability through the tunnel.
//
// A profile that routes ::/0 and has an IPv6 address says the tunnel carries
// IPv6, but some servers accept the tunnel and silently drop inner IPv6 (for
// example IVPN jp-tok-jp2). Every v6 connection then hangs with nothing coming
// back, and apps never fall back to IPv4. The probe checks what the server
// actually does, so an unusable family is blocked at once and apps use the
// other one.
const (
	// probeTimeout bounds one probe round. Every target is dialed in parallel.
	probeTimeout = 5 * time.Second

	// probeRetryInterval is how often a family marked broken, or an inconclusive
	// round, is checked again. A server can gain IPv6, and a lossy moment must
	// not block IPv6 for the rest of the session.
	probeRetryInterval = 5 * time.Minute
)

// v6Reach is what the probe learned about IPv6 through the tunnel.
type v6Reach int32

const (
	// v6Unknown means no conclusive probe has run yet. IPv6 is tunneled, as
	// the profile asks.
	v6Unknown v6Reach = iota
	// v6Usable means an IPv6 connection completed through the tunnel.
	v6Usable
	// v6Broken means IPv4 got through and IPv6 did not. IPv6 is blocked.
	v6Broken
)

// Probe targets: anycast resolvers on port 443, run by different operators so
// one outage does not look like a broken family. Only the SYN/SYN-ACK is used;
// no payload is sent. The connection leaves through the tunnel, so the targets
// see the server's exit address, not the phone's.
var (
	probeTargetsV6 = []netip.AddrPort{
		netip.MustParseAddrPort("[2606:4700:4700::1111]:443"),
		netip.MustParseAddrPort("[2001:4860:4860::8888]:443"),
		netip.MustParseAddrPort("[2620:fe::fe]:443"),
	}
	probeTargetsV4 = []netip.AddrPort{
		netip.MustParseAddrPort("1.1.1.1:443"),
		netip.MustParseAddrPort("8.8.8.8:443"),
		netip.MustParseAddrPort("9.9.9.9:443"),
	}
)

// probeState is the per-session probe bookkeeping. The supervisor reads it; the
// probe worker writes the result.
type probeState struct {
	reach   atomic.Int32
	running atomic.Bool
	// nextAt is only touched by the supervisor.
	nextAt time.Time
}

func (p *probeState) get() v6Reach { return v6Reach(p.reach.Load()) }

// maybeProbeV6 starts a probe round when one is due. Called by the supervisor
// on every connected check; it never blocks.
func (w *WireGuard) maybeProbeV6(s *session) {
	if _, v6 := s.profile.Captures(); !v6 {
		return
	}
	if s.probe.running.Load() || time.Now().Before(s.probe.nextAt) {
		return
	}
	if s.probe.get() == v6Usable {
		// Proven once for this session. A network change resets it.
		return
	}

	s.probe.running.Store(true)
	s.probe.nextAt = time.Now().Add(probeRetryInterval)
	netStack := s.net
	name := s.profile.Name
	w.mgr.Go("wireguard ipv6 probe", func(wc *mgr.WorkerCtx) error {
		defer s.probe.running.Store(false)

		reach := probeV6(wc.Ctx(), w.mgr, netStack, probeTargetsV6, probeTargetsV4)
		previous := v6Reach(s.probe.reach.Swap(int32(reach)))
		if reach != previous {
			switch reach {
			case v6Usable:
				log.Infof("wireguard: ipv6 works through %q", name)
			case v6Broken:
				log.Warningf("wireguard: %q does not carry ipv6, blocking ipv6 so apps use ipv4", name)
			case v6Unknown:
				log.Infof("wireguard: ipv6 probe through %q was inconclusive, retrying later", name)
			}
			// Publish the new family at once rather than on the next tick.
			w.nudge()
		}
		return nil
	})
}

// resetProbe forgets the result after the path changed, so the next connected
// check probes again.
func (s *session) resetProbe() {
	s.probe.reach.Store(int32(v6Unknown))
	s.probe.nextAt = time.Time{}
}

// probeV6 dials the targets of both families through the tunnel.
//
// IPv6 is only declared broken when IPv4 worked in the same round: if neither
// family gets through, the tunnel as a whole is struggling and that is the
// supervisor's business, not a reason to block one family.
func probeV6(ctx context.Context, m *mgr.Manager, netStack *netstack.Net, targetsV6, targetsV4 []netip.AddrPort) v6Reach {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	v6Ok := anyDials(ctx, m, netStack, targetsV6)
	v4Ok := anyDials(ctx, m, netStack, targetsV4)

	if <-v6Ok {
		return v6Usable
	}
	if <-v4Ok {
		return v6Broken
	}
	return v6Unknown
}

// anyDials dials every target in parallel and delivers true as soon as one TCP
// connection completes, or false once all failed or ctx ended.
func anyDials(ctx context.Context, m *mgr.Manager, netStack *netstack.Net, targets []netip.AddrPort) <-chan bool {
	results := make(chan bool, len(targets))
	for _, target := range targets {
		m.Go("wireguard probe dial", func(_ *mgr.WorkerCtx) error {
			conn, err := netStack.DialContextTCPAddrPort(ctx, target)
			if err != nil {
				results <- false
				return nil
			}
			_ = conn.Close()
			results <- true
			return nil
		})
	}

	verdict := make(chan bool, 1)
	m.Go("wireguard probe collect", func(_ *mgr.WorkerCtx) error {
		for range targets {
			if <-results {
				verdict <- true
				return nil
			}
		}
		verdict <- false
		return nil
	})
	return verdict
}

// effectiveMeta is the profile as it should be published: a family the probe
// found broken is reported as not captured, which RouteFor blocks and the app
// shows as single-family.
func (s *session) effectiveMeta() Meta {
	meta := s.profile.Meta()
	if s.probe.get() == v6Broken {
		meta.CapturesV6 = false
	}
	return meta
}
