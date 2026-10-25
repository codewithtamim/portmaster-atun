//go:build android

package process

// CleanProcessStorage cleans the storage from old processes.
// Android apps cannot enumerate other processes, so only the processes
// with active connections in activePIDs count as active.
func CleanProcessStorage(activePIDs map[int]struct{}) {
	pruneInactive(activePIDs)
}
