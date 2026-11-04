//go:build !android

package process

import (
	"time"

	processInfo "github.com/shirou/gopsutil/process"

	"github.com/safing/portmaster/base/log"
)

// CleanProcessStorage cleans the storage from old processes.
func CleanProcessStorage(activePIDs map[int]struct{}) {
	// add system table of processes
	pids, err := processInfo.Pids()
	if err != nil {
		log.Warningf("process: failed to get list of active PIDs: %s", err)
	} else {
		for _, pid := range pids {
			activePIDs[int(pid)] = struct{}{}
		}
	}

	processesCopy := All()
	threshold := time.Now().Add(-deleteProcessesThreshold).Unix()

	// clean primary processes
	for _, p := range processesCopy {
		// The PID of a process does not change.

		// Check if this is a special process.
		switch p.Pid {
		case UnidentifiedProcessID, UnsolicitedProcessID, SystemProcessID:
			p.profile.MarkStillActive()
			continue
		}

		// Check if process is active.
		_, active := activePIDs[p.Pid]
		if active {
			p.profile.MarkStillActive()
			continue
		}

		// Process is inactive, start deletion process
		lastSeen := p.GetLastSeen()
		switch {
		case lastSeen == 0:
			// add last seen timestamp
			p.SetLastSeen(time.Now().Unix())
		case lastSeen > threshold:
			// within keep period
		default:
			// delete now
			p.Delete()
			log.Tracef("process: cleaned %s", p.DatabaseKey())
		}
	}
}
