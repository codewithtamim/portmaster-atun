//go:build android

package process

import (
	"os"

	"github.com/safing/portmaster/service/profile"
)

// ownUID is the UID this app runs as. On Android it is the identity of our own
// traffic, the way ownPID is on the desktop platforms.
var ownUID = os.Getuid()

// Android platform UIDs.
const (
	// rootUID is the UID of root. Only reachable on rooted devices.
	rootUID = 0
	// androidSystemUID is the UID of the Android system server (android.uid.system).
	androidSystemUID = 1000
	// androidDNSUID is the UID of Android's DNS resolver (AID_DNS). It makes
	// lookups for Android itself, such as checking that Private DNS works;
	// apps' own lookups carry the app's UID.
	androidDNSUID = 1051
)

// IsAndroidSystemUID reports whether uid is a service of Android itself, so
// its traffic belongs to the operating system even when, like the DNS
// resolver, it has no app package to look up. Root is left out: traffic of
// root processes on a rooted phone stays unidentified.
func IsAndroidSystemUID(uid int) bool {
	return uid == androidSystemUID || uid == androidDNSUID
}

// getSpecialProfileID returns the special profile ID for the app, if any.
//
// Android apps are identified by their UID, not by a PID, so this switches on
// the UID. The special processes in special.go set UserID equal to Pid, which
// keeps them mapping correctly here.
func (p *Process) getSpecialProfileID() (specialProfileID string) {
	switch p.UserID {
	case UndefinedProcessID, UnidentifiedProcessID:
		specialProfileID = profile.UnidentifiedProfileID
	case UnsolicitedProcessID:
		specialProfileID = profile.UnsolicitedProfileID
	case rootUID:
		specialProfileID = profile.SystemProfileID
	case ownUID:
		specialProfileID = profile.PortmasterProfileID
	default:
		if IsAndroidSystemUID(p.UserID) {
			specialProfileID = profile.SystemProfileID
		}
	}

	return specialProfileID
}

// hasIdentifiedOwner returns whether the app is a real app and not one of the
// special unidentified placeholders.
func (p *Process) hasIdentifiedOwner() bool {
	switch p.UserID {
	case UndefinedProcessID, UnidentifiedProcessID, UnsolicitedProcessID:
		return false
	default:
		return p.UserID >= 0
	}
}

// ownerID returns the identity of the process owner on this platform.
func (p *Process) ownerID() int {
	return p.UserID
}
