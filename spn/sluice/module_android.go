//go:build android

package sluice

// No listener on Android. TUN hands conns to crew direct.
func init() {
	EnableListener = false
}
