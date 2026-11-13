//go:build android

package access

import "github.com/safing/portmaster/service/mgr"

// AccountChanged returns a channel that receives once, at the next sign-in,
// sign-out or account refresh.
func AccountChanged() <-chan struct{} {
	changed := make(chan struct{}, 1)
	module.EventAccountUpdate.AddCallback("notify account change", func(*mgr.WorkerCtx, struct{}) (bool, error) {
		select {
		case changed <- struct{}{}:
		default:
		}
		// One change is all the waiter needs.
		return true, nil
	})
	return changed
}
