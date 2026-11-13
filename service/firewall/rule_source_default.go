//go:build !android

package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile"
)

// noteRuleSource keeps the verdict's profile as it is on desktop.
func noteRuleSource(context.Context, *network.Connection, *profile.LayeredProfile) {}
