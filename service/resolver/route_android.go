//go:build android

package resolver

import (
	"net"
	"net/http"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/routing"
	"github.com/safing/portmaster/service/wireguard"
)

func wireguardResolver() bool {
	return routing.Current() == routing.TargetWireGuard
}

func routeResolverDialer(d *net.Dialer) *net.Dialer {
	// When WireGuard is the route, the TUN carries the dial, so the plain
	// dialer is enough. Otherwise protect the socket so it bypasses the VPN.
	if wireguardResolver() {
		return d
	}
	return aprotect.ApplyToDialer(d)
}

func routeResolverTransport(tr *http.Transport) *http.Transport {
	if wireguardResolver() {
		tr.DialContext = wireguard.DialContext
		return tr
	}
	return aprotect.ApplyToTransport(tr)
}

// reconnectOnRouteChange makes the resolvers redial when the routing target
// changes. DoH and DoT pick the route when they connect, so their open
// connections would otherwise keep the previous route.
func reconnectOnRouteChange() {
	ri, ok := module.instance.(interface{ Routing() *routing.Routing })
	if !ok {
		return
	}
	ri.Routing().EventTargetChanged.AddCallback(
		"force resolver reconnect",
		func(wc *mgr.WorkerCtx, target routing.Target) (bool, error) {
			ForceResolverReconnect(wc.Ctx())
			log.Debugf("resolver: forced reconnect after routing target changed to %s", target)
			return false, nil
		},
	)
}
