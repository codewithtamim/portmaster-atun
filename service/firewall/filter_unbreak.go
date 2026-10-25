package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile/endpoints"
)

// concludeUnbreakMatch leaves the verdict unset so the default action applies.
// An unbreak-list match only cancels the filter-list block; it does not allow
// the connection on its own.
func concludeUnbreakMatch(_ context.Context, _ *network.Connection, _ endpoints.Reason, _ string) bool {
	return false
}
