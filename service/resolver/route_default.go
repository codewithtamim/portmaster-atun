//go:build !android

package resolver

import (
	"net"
	"net/http"

	"github.com/safing/portmaster/base/utils/aprotect"
)

func routeResolverDialer(d *net.Dialer) *net.Dialer {
	return aprotect.ApplyToDialer(d)
}

func routeResolverTransport(tr *http.Transport) *http.Transport {
	return aprotect.ApplyToTransport(tr)
}

func reconnectOnRouteChange() {}
