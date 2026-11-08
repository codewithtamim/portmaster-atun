//go:build !android

package resolver

import (
	"context"
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

func spnLockdownGate(ctx context.Context, _ *Query, _ *Resolver) (context.Context, error) {
	return ctx, nil
}
