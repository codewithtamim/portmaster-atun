//go:build android

package crew

import (
	"context"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/spn/navigator"
)

// hopProfile is the hop profile a route was requested for.
type hopProfile = string

// fitStickiedHub records the hop profile selected for this connection and
// drops a remembered route built for a different one. The profile is captured
// before FindRoutes rewrites it: a home hub without the destination IP family
// is upgraded to single-hop in place.
func fitStickiedHub(ctx context.Context, t *Tunnel, sticksTo *stickyHub) *stickyHub {
	if t.connInfo.TunnelOpts != nil {
		t.requestedProfile = t.connInfo.TunnelOpts.RoutingProfile
	}
	if sticksTo != nil && !sticksTo.Avoid && !stickyRouteFits(sticksTo, t.requestedProfile) {
		log.Tracer(ctx).Tracef("spn/crew: stickied route does not match %s", t.requestedProfile)
		return nil
	}
	return sticksTo
}

// stickyRouteFits reports whether a remembered route was built for the hop
// profile this connection is using. The comparison uses the profile that was
// requested, not the algorithm the finder may have upgraded it to. A home hub
// without the destination IP family becomes a single-hop route in place, and
// that route still belongs to the home profile.
func stickyRouteFits(sticksTo *stickyHub, requestedProfile string) bool {
	if sticksTo == nil || sticksTo.Route == nil || len(sticksTo.Route.Path) == 0 {
		return false
	}
	return sticksTo.Profile == requestedProfile
}

// ClearStickyHubs drops remembered routes. A stickied route is reused for an
// hour without asking the route finder, so a hop change would otherwise keep
// sending new connections down the old path.
func ClearStickyHubs() {
	clearStickyHubs()
}

// NoteRoute, if set, receives every route crew establishes: the real hop
// count and the exit hub's country. The routing status sets it.
var NoteRoute func(hops int, exitCountry string)

func publishEstablishedRoute(route *navigator.Route, pin *navigator.Pin) {
	if NoteRoute == nil || route == nil {
		return
	}
	NoteRoute(len(route.Path), pinCountryName(pin))
}

func pinCountryName(pin *navigator.Pin) string {
	if pin == nil {
		return ""
	}
	if pin.LocationV4 != nil && pin.LocationV4.Country.Name != "" {
		return pin.LocationV4.Country.Name
	}
	if pin.LocationV6 != nil {
		return pin.LocationV6.Country.Name
	}
	return ""
}
