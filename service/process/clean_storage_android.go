//go:build android

package process

import (
	"time"

	"github.com/safing/portmaster/base/log"
)

// CleanProcessStorage cleans the storage from old processes.
// Android apps cannot enumerate other processes, so only the processes
// with active connections in activePIDs count as active. Processes are
// keyed by app UID, so the special IDs are checked against the UID.
func CleanProcessStorage(activePIDs map[int]struct{}) {
	processesCopy := All()
	threshold := time.Now().Add(-deleteProcessesThreshold).Unix()

	for _, p := range processesCopy {
		// Check if this is a special process.
		switch p.ownerID() {
		case UnidentifiedProcessID, UnsolicitedProcessID, SystemProcessID:
			p.profile.MarkStillActive()
			continue
		}

		// Check if process is active.
		if _, active := activePIDs[p.Pid]; active {
			p.profile.MarkStillActive()
			continue
		}

		// Process is inactive, start deletion process.
		lastSeen := p.GetLastSeen()
		switch {
		case lastSeen == 0:
			// Add last seen timestamp.
			p.SetLastSeen(time.Now().Unix())
		case lastSeen > threshold:
			// Within keep period.
		default:
			// Delete now.
			p.Delete()
			log.Tracef("process: cleaned %s", p.DatabaseKey())
		}
	}
}
