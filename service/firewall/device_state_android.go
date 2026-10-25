//go:build android

package firewall

import "github.com/safing/portmaster/base/utils/aconn"

func init() {
	aconn.SetOnDeviceStateChange(func() {
		if module == nil {
			return
		}
		resetAllConnectionVerdicts()
	})
}
