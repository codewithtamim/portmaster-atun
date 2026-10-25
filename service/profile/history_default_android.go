//go:build android

package profile

import "github.com/safing/portmaster/base/config"

// applyAndroidHistoryDefault records history on a fresh install.
// An explicit user value still wins. Other platforms keep the registered default.
func applyAndroidHistoryDefault() error {
	return config.SetDefaultConfigOption(CfgOptionEnableHistoryKey, true)
}
