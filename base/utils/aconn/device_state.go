package aconn

import (
	"encoding/json"
	"sync"
)

// DeviceState is the Android device policy input: which network is
// the default route, whether the keyguard is showing, and whether the
// active network is a captive portal. Captive and PortalURL stay zero
// unless Android pushes them, so other platforms are unchanged.
type DeviceState struct {
	Wifi      bool   `json:"wifi"`
	Cellular  bool   `json:"cellular"`
	Locked    bool   `json:"locked"`
	Captive   bool   `json:"captive"`
	PortalURL string `json:"portalUrl"`
	Metered   bool   `json:"metered"`
}

var (
	deviceStateMu sync.RWMutex
	deviceState   DeviceState
)

// UpdateDeviceStateJSON replaces the cached network, lock and captive flags.
func UpdateDeviceStateJSON(jsonStr string) error {
	var next DeviceState
	if jsonStr != "" {
		err := json.Unmarshal([]byte(jsonStr), &next)
		if err != nil {
			return err
		}
	}
	deviceStateMu.Lock()
	changed := deviceState != next
	deviceState = next
	deviceStateMu.Unlock()
	if changed {
		notifyDeviceStateChange()
	}
	return nil
}

// CurrentDeviceState returns the last pushed device flags.
func CurrentDeviceState() DeviceState {
	deviceStateMu.RLock()
	defer deviceStateMu.RUnlock()
	return deviceState
}
