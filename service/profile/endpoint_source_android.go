//go:build android

package profile

import (
	"context"

	"github.com/safing/portmaster/service/intel"
	"github.com/safing/portmaster/service/profile/endpoints"
)

// EndpointRuleSource returns the key of the profile layer whose rules decide
// entity, or the global configuration profile's key when no layer does and the
// global rules decide. It repeats
// the layer walk of MatchEndpoint and MatchServiceEndpoint, so the app can
// say whether an app's own rule or a rule for all apps made the decision.
// This function requires the layered profile to be read locked.
func (lp *LayeredProfile) EndpointRuleSource(ctx context.Context, entity *intel.Entity, inbound bool) string {
	for _, layer := range lp.layers {
		rules := layer.endpoints
		if inbound {
			rules = layer.serviceEndpoints
		}
		if !rules.IsSet() {
			continue
		}
		if result, _ := rules.Match(ctx, entity); endpoints.IsDecision(result) {
			return layer.Key()
		}
	}
	return MakeProfileKey(SourceSpecial, globalConfigProfileID)
}
