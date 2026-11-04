//go:build !android

package crew

import (
	"context"

	"github.com/safing/portmaster/spn/navigator"
)

// hopProfile is empty here, as remembered routes are not tied to a hop profile.
type hopProfile struct{}

// fitStickiedHub returns the remembered route unchanged.
func fitStickiedHub(_ context.Context, _ *Tunnel, sticksTo *stickyHub) *stickyHub {
	return sticksTo
}

// publishEstablishedRoute does nothing here, as nothing shows the live route.
func publishEstablishedRoute(_ *navigator.Route, _ *navigator.Pin) {}
