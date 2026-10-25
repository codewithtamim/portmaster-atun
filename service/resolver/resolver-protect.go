package resolver

import (
	"net"
	"net/http"
)

// resolverShouldUseSPN is true only when the user wants SPN lookups and SPN
// is actually connected. Otherwise we keep protecting sockets so DNS still
// works without VPN/SPN, matching "only applies when your route is SPN".
func resolverShouldUseSPN() bool {
	if useSpnLookups == nil || !useSpnLookups() {
		return false
	}
	if module == nil || module.instance == nil {
		return false
	}
	return module.instance.SPNReady()
}

// resolverDialer routes a resolver socket: through the SPN when SPN lookups
// are on and SPN is ready, otherwise by the routing target.
func resolverDialer(d *net.Dialer) *net.Dialer {
	if resolverShouldUseSPN() {
		return d
	}
	return routeResolverDialer(d)
}

// resolverTransport wraps an HTTP transport the same way for DoH.
func resolverTransport(tr *http.Transport) *http.Transport {
	if resolverShouldUseSPN() {
		return tr
	}
	return routeResolverTransport(tr)
}
