//go:build android

package netenv

import "time"

// getDynamicStatusTrigger is upstream's, except for a portal or limited
// network. Upstream probes those every 5 seconds, because nothing else would
// notice a change. On Android each probe is an HTTP request that wakes the
// radio, and a phone can sit on a hotel Wi-Fi it never signed into all night.
// Android detects portals itself and pushes the change (ApplyAndroidCaptive),
// which starts an investigation right away, so these checks only cover what
// Android does not report.
func getDynamicStatusTrigger() <-chan time.Time {
	interval, ok := statusCheckInterval(GetOnlineStatus())
	if !ok {
		return nil
	}
	return time.After(interval)
}

// statusCheckInterval returns how long to wait before checking status again,
// or false to wait for an event.
func statusCheckInterval(status OnlineStatus) (time.Duration, bool) {
	switch status {
	case StatusOffline:
		// A pushed network change does not start an investigation (see
		// monitorNetworkChanges), so this is how coming back online is noticed.
		// With no address the check makes no request.
		return 10 * time.Second, true
	case StatusPortal:
		// Signing in clears the portal on Android, which is pushed.
		return 60 * time.Second, true
	case StatusLimited:
		return 30 * time.Second, true
	case StatusSemiOnline:
		// Very small impact.
		return 60 * time.Second, true
	case StatusOnline:
		// Don't check until resolver reports problems.
		return 0, false
	case StatusUnknown:
		fallthrough
	default:
		return 5 * time.Minute, true
	}
}
