//go:build !android

package netenv

// ApplyAndroidCaptive is a no-op off Android. Desktop online status stays
// on the existing captive-portal detector.
func ApplyAndroidCaptive(_ bool, _ string) {}

// CaptivePortalAPI is the body of GET network/captive. The endpoint is not
// registered off Android.
type CaptivePortalAPI struct {
	Active bool   `json:"active"`
	URL    string `json:"url"`
}

// CaptivePortalStatus reports no portal off Android.
func CaptivePortalStatus() CaptivePortalAPI {
	return CaptivePortalAPI{}
}

func registerCaptivePortalAPI() error { return nil }
