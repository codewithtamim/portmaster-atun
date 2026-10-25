//go:build !android

package profile

func registerAndroidDevicePolicyOptions() error {
	return nil
}

// preventBypassingDefault stays on outside Android, matching the desktop
// "Block Secure DNS Bypassing" option.
func preventBypassingDefault() bool { return true }

func applyAndroidDevicePolicyOptions(lp *LayeredProfile) {
	lp.BlockOnMobile = func() bool { return false }
	lp.BlockOnLock = func() bool { return false }
	lp.BlockOnWifi = func() bool { return false }
}
