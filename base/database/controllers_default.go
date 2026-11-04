//go:build !android

package database

// startedWhileWaiting never finds a controller: desktop keeps the upstream
// behavior.
// TODO: two callers that miss the read-locked lookup at the same time both
// open the storage, and the second open can block while holding
// controllersLock. Desktop has the same race.
func startedWhileWaiting(string) (*Controller, bool) { return nil, false }
