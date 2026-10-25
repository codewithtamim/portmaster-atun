package process

import (
	"time"

	"github.com/safing/portmaster/base/log"
)

// pruneInactive deletes processes whose Pid is not in activePIDs and that
// have not been seen within deleteProcessesThreshold.
func pruneInactive(activePIDs map[int]struct{}) {
	processesCopy := All()
	threshold := time.Now().Add(-deleteProcessesThreshold).Unix()

	for _, p := range processesCopy {
		// The PID of a process does not change.
		switch p.ownerID() {
		case UnidentifiedProcessID, UnsolicitedProcessID, SystemProcessID:
			p.profile.MarkStillActive()
			continue
		}

		if _, active := activePIDs[p.Pid]; active {
			p.profile.MarkStillActive()
			continue
		}

		lastSeen := p.GetLastSeen()
		switch {
		case lastSeen == 0:
			// Add last seen timestamp.
			p.SetLastSeen(time.Now().Unix())
		case lastSeen > threshold:
			// Within keep period.
		default:
			p.Delete()
			log.Tracef("process: cleaned %s", p.DatabaseKey())
		}
	}
}
