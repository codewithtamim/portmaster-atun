//go:build android

package compat

// No selfcheck on Android. No raw sockets. It would always fail.
func selfcheckSupported() bool {
	return false
}
