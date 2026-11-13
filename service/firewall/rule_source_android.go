//go:build android

package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile"
)

// noteRuleSource records which profile layer's rule decided conn. The verdict
// otherwise names the first layer that has any rules, so an app with rules of
// its own would be shown as deciding a connection that a rule for all apps
// decided.
func noteRuleSource(ctx context.Context, conn *network.Connection, p *profile.LayeredProfile) {
	conn.Reason.Profile = p.EndpointRuleSource(ctx, conn.Entity, conn.Inbound)
}
