//go:build android

package crew

import (
	"testing"

	"github.com/safing/portmaster/spn/navigator"
)

func TestStickyRouteFitsRequestedProfile(t *testing.T) {
	homeUpgraded := &stickyHub{
		RequestedProfile: navigator.RoutingProfileHomeID,
		Route: &navigator.Route{
			Algorithm: navigator.RoutingProfileSingleHopID,
			Path:      []*navigator.Hop{{}, {}},
		},
	}
	if !stickyRouteFits(homeUpgraded, navigator.RoutingProfileHomeID) {
		t.Fatal("a home route upgraded to single-hop must still be reused for home")
	}
	if stickyRouteFits(homeUpgraded, navigator.RoutingProfileDoubleHopID) {
		t.Fatal("a home route must not be reused after a different hop profile is selected")
	}

	double := &stickyHub{
		RequestedProfile: navigator.RoutingProfileDoubleHopID,
		Route: &navigator.Route{
			Algorithm: navigator.RoutingProfileDoubleHopID,
			Path:      []*navigator.Hop{{}, {}},
		},
	}
	if !stickyRouteFits(double, navigator.RoutingProfileDoubleHopID) {
		t.Fatal("a route that still matches the selected profile should be reused")
	}
	if stickyRouteFits(double, navigator.RoutingProfileSingleHopID) {
		t.Fatal("a double-hop route must not be reused after fewer hops is selected")
	}
	if stickyRouteFits(&stickyHub{RequestedProfile: navigator.RoutingProfileHomeID}, navigator.RoutingProfileHomeID) {
		t.Fatal("a stickied hub with no route must not be reused")
	}
}
