//go:build !android

package nameserver

import (
	"context"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/resolver"
)

// synthesizeAAAA is Android only. Everywhere else the answer stays as resolved.
func synthesizeAAAA(_ context.Context, _ *network.Connection, _ *resolver.Query, rrCache *resolver.RRCache) *resolver.RRCache {
	return rrCache
}
