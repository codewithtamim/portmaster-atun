//go:build !android

package process

import (
	processInfo "github.com/shirou/gopsutil/process"

	"github.com/safing/portmaster/base/log"
)

// CleanProcessStorage cleans the storage from old processes.
func CleanProcessStorage(activePIDs map[int]struct{}) {
	// Add system table of processes.
	pids, err := processInfo.Pids()
	if err != nil {
		log.Warningf("process: failed to get list of active PIDs: %s", err)
	} else {
		for _, pid := range pids {
			activePIDs[int(pid)] = struct{}{}
		}
	}

	pruneInactive(activePIDs)
}
