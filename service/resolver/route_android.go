//go:build android

package resolver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/safing/portmaster/base/database"
	"github.com/safing/portmaster/base/database/query"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/netenv"
	"github.com/safing/portmaster/service/network/netutils"
	"github.com/safing/portmaster/service/routing"
	"github.com/safing/portmaster/service/wireguard"
)

// ErrSPNLockdown is returned while the route is SPN, routing lockdown is on
// and the SPN is not ready. Lookups wait for the SPN instead of going out
// directly.
var ErrSPNLockdown = errors.New("resolver: SPN not ready, lookups are held by routing lockdown")

// resolverRoute is where a resolver socket goes.
type resolverRoute int

const (
	// routeProtected bypasses the VPN and leaves from the real network.
	routeProtected resolverRoute = iota
	// routeSPN enters the TUN, and the firewall tunnels it through the SPN.
	routeSPN
	// routeWireGuard goes through the WireGuard tunnel.
	routeWireGuard
	// routeBlocked fails the dial.
	routeBlocked
)

// routeState holds the inputs of the routing decision.
type routeState struct {
	useSPNLookups bool
	spnReady      bool
	target        routing.Target
	lockdown      bool
}

func currentRouteState() routeState {
	s := routeState{
		target:   routing.Current(),
		lockdown: routing.Lockdown(),
	}
	if useSpnLookups != nil {
		s.useSPNLookups = useSpnLookups()
	}
	if module != nil && module.instance != nil {
		s.spnReady = module.instance.SPNReady()
	}
	return s
}

// holdsLookups reports whether lookups must wait for the SPN.
func (s routeState) holdsLookups() bool {
	return s.target == routing.TargetSPN && s.useSPNLookups && !s.spnReady && s.lockdown
}

// route picks the route of one dial. Local resolvers and exempt lookups are
// never held: they would not use the SPN anyway, or the SPN needs them to
// connect.
func (s routeState) route(ctx context.Context, address string) resolverRoute {
	switch {
	case s.useSPNLookups && s.spnReady:
		return routeSPN
	case s.target == routing.TargetWireGuard:
		return routeWireGuard
	case s.holdsLookups() && !lockdownExempt(ctx) && globalAddress(address):
		return routeBlocked
	default:
		return routeProtected
	}
}

// globalAddress reports whether address is a global IP. Anything that is
// not an IP literal counts as global.
func globalAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return netutils.GetIPScope(ip).IsGlobal()
}

type lockdownExemptKey struct{}

// withLockdownExempt marks a query whose dial may go out directly while
// lookups are held.
func withLockdownExempt(ctx context.Context) context.Context {
	return context.WithValue(ctx, lockdownExemptKey{}, true)
}

func lockdownExempt(ctx context.Context) bool {
	exempt, _ := ctx.Value(lockdownExemptKey{}).(bool)
	return exempt
}

// lockdownExemptDomains are Portmaster's own servers. The SPN needs them to
// log in and get access tokens, so holding them could keep it down for good.
var lockdownExemptDomains = []string{
	"safing.io.",
	"safing.net.",
}

// lockdownExemptDomain reports whether a lookup for fqdn may go out directly
// while lookups are held: the online check, connectivity checks and
// Portmaster's own servers. The SPN does not connect while offline.
func lockdownExemptDomain(fqdn string) bool {
	fqdn = strings.ToLower(fqdn)
	if fqdn == netenv.DNSTestDomain || netenv.IsConnectivityDomain(fqdn) {
		return true
	}
	for _, domain := range lockdownExemptDomains {
		if fqdn == domain || strings.HasSuffix(fqdn, "."+domain) {
			return true
		}
	}
	return false
}

// spnLockdownGate holds a query back from a resolver while the SPN is down
// and routing lockdown is on. Exempt queries get a marked context, so their
// dial is let through. Connections opened by them stay unused by other
// queries, as those are held here, and are dropped once the SPN connects.
func spnLockdownGate(ctx context.Context, q *Query, resolver *Resolver) (context.Context, error) {
	return currentRouteState().gate(ctx, q, resolver)
}

func (s routeState) gate(ctx context.Context, q *Query, resolver *Resolver) (context.Context, error) {
	if !s.holdsLookups() {
		return ctx, nil
	}
	switch {
	case resolver.Info.Source == ServerSourceEnv,
		resolver.Info.Source == ServerSourceMDNS,
		!resolver.Info.IPScope.IsGlobal():
		return ctx, nil
	case lockdownExemptDomain(q.FQDN):
		return withLockdownExempt(ctx), nil
	default:
		return ctx, ErrSPNLockdown
	}
}

func routeResolverDialer(d *net.Dialer) *net.Dialer {
	prevControlContext := d.ControlContext
	prevControl := d.Control
	d.Control = nil
	d.ControlContext = func(ctx context.Context, network, address string, c syscall.RawConn) error {
		switch currentRouteState().route(ctx, address) {
		case routeBlocked:
			return ErrSPNLockdown
		case routeProtected:
			if err := protectRawConn(c); err != nil {
				return err
			}
		case routeSPN, routeWireGuard:
			// The TUN carries the socket.
		}
		switch {
		case prevControlContext != nil:
			return prevControlContext(ctx, network, address, c)
		case prevControl != nil:
			return prevControl(network, address, c)
		default:
			return nil
		}
	}
	return d
}

var errProtectFailed = errors.New("resolver: failed to protect socket")

func protectRawConn(c syscall.RawConn) error {
	protected := true
	if err := c.Control(func(fd uintptr) {
		protected = aprotect.ProtectFD(int(fd))
	}); err != nil {
		return err
	}
	if !protected {
		return errProtectFailed
	}
	return nil
}

func routeResolverTransport(tr *http.Transport) *http.Transport {
	// Same timeouts as the dialer of http.DefaultTransport.
	plain := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	protected := aprotect.ApplyToDialer(&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch currentRouteState().route(ctx, address) {
		case routeBlocked:
			return nil, ErrSPNLockdown
		case routeSPN:
			return plain.DialContext(ctx, network, address)
		case routeWireGuard:
			return wireguard.DialContext(ctx, network, address)
		default:
			return protected.DialContext(ctx, network, address)
		}
	}
	return tr
}

// reconnectOnRouteChange makes the resolvers redial when the routing target
// changes or the SPN disconnects. DoH and DoT keep their open connections,
// which would otherwise stay on the previous route.
func reconnectOnRouteChange() {
	ri, ok := module.instance.(interface{ Routing() *routing.Routing })
	if ok {
		ri.Routing().EventTargetChanged.AddCallback(
			"force resolver reconnect",
			func(wc *mgr.WorkerCtx, target routing.Target) (bool, error) {
				ForceResolverReconnect(wc.Ctx())
				log.Debugf("resolver: forced reconnect after routing target changed to %s", target)
				return false, nil
			},
		)
	}

	module.instance.GetEventSPNConnected().AddCallback(
		"track spn ready",
		func(_ *mgr.WorkerCtx, _ struct{}) (bool, error) {
			spnWasReady.Store(true)
			return false, nil
		},
	)
	module.mgr.Go("reconnect resolvers on spn disconnect", watchSPNDisconnect)
}

// spnWasReady is the SPN state last seen by watchSPNDisconnect.
var spnWasReady atomic.Bool

var spnStatusDB = database.NewInterface(&database.Options{Local: true, Internal: true})

// watchSPNDisconnect forces a reconnect when the SPN goes down. The SPN has
// no disconnect event, but it pushes its status on every change.
func watchSPNDisconnect(wc *mgr.WorkerCtx) error {
	sub, err := spnStatusDB.Subscribe(query.New("runtime:spn/status"))
	if err != nil {
		log.Warningf("resolver: cannot watch the spn status: %s", err)
		return nil
	}
	defer func() {
		_ = sub.Cancel()
	}()

	spnWasReady.Store(module.instance.SPNReady())
	for {
		select {
		case <-wc.Done():
			return nil
		case r := <-sub.Feed:
			if r == nil {
				return nil
			}
			if spnDisconnected(module.instance.SPNReady()) {
				ForceResolverReconnect(wc.Ctx())
				log.Debug("resolver: forced reconnect after spn disconnected")
			}
		}
	}
}

// spnDisconnected records the SPN state and reports a ready to not ready
// change.
func spnDisconnected(ready bool) bool {
	return spnWasReady.Swap(ready) && !ready
}
