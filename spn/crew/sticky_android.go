//go:build android

package crew

// stickyRouteFits reports whether a remembered route was built for the hop
// profile this connection is using. The comparison uses the profile that was
// requested, not the algorithm the finder may have upgraded it to. A home hub
// without the destination IP family becomes a single-hop route in place, and
// that route still belongs to the home profile.
func stickyRouteFits(sticksTo *stickyHub, requestedProfile string) bool {
	if sticksTo == nil || sticksTo.Route == nil || len(sticksTo.Route.Path) == 0 {
		return false
	}
	return sticksTo.RequestedProfile == requestedProfile
}
