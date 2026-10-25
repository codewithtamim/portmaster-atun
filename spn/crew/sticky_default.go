//go:build !android

package crew

// stickyRouteFits always accepts a remembered route on desktop. Reuse for an
// hour is the behavior from before the Android hop-profile check.
func stickyRouteFits(_ *stickyHub, _ string) bool {
	return true
}
