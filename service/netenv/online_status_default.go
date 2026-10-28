//go:build !android

package netenv

import "time"

func getDynamicStatusTrigger() <-chan time.Time {
	switch GetOnlineStatus() {
	case StatusOffline:
		// Will also be triggered by network change.
		return time.After(10 * time.Second)
	case StatusLimited, StatusPortal:
		// Change will not be detected otherwise, but impact is minor.
		return time.After(5 * time.Second)
	case StatusSemiOnline:
		// Very small impact.
		return time.After(60 * time.Second)
	case StatusOnline:
		// Don't check until resolver reports problems.
		return nil
	case StatusUnknown:
		fallthrough
	default:
		return time.After(5 * time.Minute)
	}
}
