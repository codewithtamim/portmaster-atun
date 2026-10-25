//go:build android

package profile

import "testing"

func TestRoutingAlgorithmChangeRefreshesLayeredProfile(t *testing.T) {
	previous := lastRoutingAlgorithm
	t.Cleanup(func() {
		lastRoutingAlgorithm = previous
		activeProfilesLock.Lock()
		delete(activeProfiles, MakeScopedID(SourceLocal, "algo-app"))
		activeProfilesLock.Unlock()
	})
	lastRoutingAlgorithm = "double-hop"

	algo := "double-hop"
	lp := &LayeredProfile{RevisionCounter: 1}
	lp.SPNRoutingAlgorithm = lp.wrapStringOption(CfgOptionRoutingAlgorithmKey, func() string { return algo })
	if got := lp.SPNRoutingAlgorithm(); got != "double-hop" {
		t.Fatalf("initial algorithm: %s", got)
	}

	active := &Profile{
		ID:             "algo-app",
		Source:         SourceLocal,
		layeredProfile: lp,
	}
	activeProfilesLock.Lock()
	activeProfiles[active.ScopedID()] = active
	activeProfilesLock.Unlock()

	algo = "triple-hop"
	if got := lp.SPNRoutingAlgorithm(); got != "double-hop" {
		t.Fatalf("cached algorithm: %s", got)
	}
	refreshLayeredProfilesForRoutingAlgorithm("triple-hop")
	if got := lp.SPNRoutingAlgorithm(); got != "triple-hop" {
		t.Fatalf("after triple-hop: %s", got)
	}

	algo = "single-hop"
	refreshLayeredProfilesForRoutingAlgorithm("single-hop")
	if got := lp.SPNRoutingAlgorithm(); got != "single-hop" {
		t.Fatalf("after single-hop: %s", got)
	}
}
