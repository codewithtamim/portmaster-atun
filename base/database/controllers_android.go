//go:build android

package database

// startedWhileWaiting returns the controller another caller started while
// this one waited for controllersLock. Opening the storage a second time
// would fail or block, since Android opens databases from several modules at
// once during start. The caller holds controllersLock.
func startedWhileWaiting(name string) (*Controller, bool) {
	controller, ok := controllers[name]
	return controller, ok
}
