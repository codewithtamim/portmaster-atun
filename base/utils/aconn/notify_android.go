//go:build android

package aconn

import "sync"

var (
	deviceStateHandlerMu sync.RWMutex
	deviceStateHandler   func()
)

// SetOnDeviceStateChange registers the callback run after the cached device
// flags actually change. The firewall uses it to re-evaluate open connections.
func SetOnDeviceStateChange(fn func()) {
	deviceStateHandlerMu.Lock()
	deviceStateHandler = fn
	deviceStateHandlerMu.Unlock()
}

func notifyDeviceStateChange() {
	deviceStateHandlerMu.RLock()
	fn := deviceStateHandler
	deviceStateHandlerMu.RUnlock()
	if fn != nil {
		fn()
	}
}
