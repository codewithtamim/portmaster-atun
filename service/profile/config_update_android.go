//go:build android

package profile

import "sync"

// lastRoutingAlgorithm is the SPN hop profile that layered profiles last saw.
// wrapStringOption caches spn/routingAlgorithm until RevisionCounter moves,
// and a global config write does not touch app profile records, so without
// this a hop change on the Android routing screen never reaches running apps.
//
// Do not guard this with cfgLock. Firewall checks take cfgLock while holding a
// layered profile read lock, so taking the profile lock under cfgLock deadlocks.
var (
	lastRoutingAlgorithm   string
	lastRoutingAlgorithmMu sync.Mutex
)

// refreshLayeredProfilesForRoutingAlgorithm bumps the revision of every active
// layered profile when the global hop profile changes, so the next
// SPNRoutingAlgorithm call re-reads it.
func refreshLayeredProfilesForRoutingAlgorithm(next string) {
	lastRoutingAlgorithmMu.Lock()
	if next == lastRoutingAlgorithm {
		lastRoutingAlgorithmMu.Unlock()
		return
	}
	lastRoutingAlgorithm = next
	lastRoutingAlgorithmMu.Unlock()

	for _, active := range getAllActiveProfiles() {
		if active.layeredProfile != nil {
			active.layeredProfile.increaseRevisionCounter(true)
		}
	}
}
