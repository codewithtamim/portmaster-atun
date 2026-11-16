//go:build android

package firewall

import "github.com/safing/portmaster/service/profile"

// sharesIPInfoGlobally reports whether lookups made under this profile are
// stored for every app. On Android, apps' DNS often reaches the engine from
// the system resolver under the system identity, so the app that asked is not
// known. Storing those answers globally lets the app's later connection to
// the address find its domain, so domain rules and filter lists apply to it.
func sharesIPInfoGlobally(profileID string) bool {
	return profileID == profile.SystemProfileID
}
