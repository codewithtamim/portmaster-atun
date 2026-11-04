//go:build android

package log

// ClearLastUnexpectedLogs empties the in-memory unexpected-log ring.
func ClearLastUnexpectedLogs() {
	lastUnexpectedLogsLock.Lock()
	defer lastUnexpectedLogsLock.Unlock()

	for i := range lastUnexpectedLogs {
		lastUnexpectedLogs[i] = ""
	}
	lastUnexpectedLogsIndex = 0
}
