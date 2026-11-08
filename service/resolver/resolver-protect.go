package resolver

import (
	"net"
	"net/http"
)

// resolverDialer routes a resolver socket. On Android the route is picked at
// every dial, see route_android.go. Elsewhere the socket is protected as usual.
func resolverDialer(d *net.Dialer) *net.Dialer {
	return routeResolverDialer(d)
}

// resolverTransport wraps an HTTP transport the same way for DoH.
func resolverTransport(tr *http.Transport) *http.Transport {
	return routeResolverTransport(tr)
}
