//go:build android

package firewall

import "github.com/safing/portmaster/base/utils/aconn"

// startDeviceStateWatch re-evaluates open connections when the device state
// that device policies depend on changes.
func startDeviceStateWatch() {
	aconn.SetOnDeviceStateChange(resetAllConnectionVerdicts)
}

func stopDeviceStateWatch() {
	aconn.SetOnDeviceStateChange(nil)
}
