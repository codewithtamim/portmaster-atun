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
)

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
	case rootUID, androidSystemUID:
		specialProfileID = profile.SystemProfileID
	case ownUID:
		specialProfileID = profile.PortmasterProfileID
	}

	// There is no system resolver to detect: on Android DNS goes through the
	// system server, which is already covered by androidSystemUID above.

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
