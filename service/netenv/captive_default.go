//go:build !android

package netenv

// ApplyAndroidCaptive is a no-op off Android. Desktop online status stays
// on the existing captive-portal detector.
func ApplyAndroidCaptive(_ bool, _ string) {}

func registerCaptivePortalAPI() error { return nil }
