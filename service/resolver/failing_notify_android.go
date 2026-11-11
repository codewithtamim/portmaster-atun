//go:build android

package resolver

import "github.com/safing/portmaster/base/utils/anotif"

// The app shows engine warnings only through anotif, so the warning that all
// configured DNS servers fail is passed on there as well. The app words it
// itself; title and message only need to be non-empty.
func platformNotifyFailingResolvers() {
	anotif.Deliver(failingResolverErrorID, "Configured DNS servers failing", "All configured DNS servers are failing.")
}

// platformResetFailingResolvers runs after every answered global query, so it
// only calls into the app when the warning is actually shown.
func platformResetFailingResolvers() {
	if !anotif.AlreadyDelivered(failingResolverErrorID) {
		return
	}
	anotif.Retract(failingResolverErrorID)
}
