//go:build android

package base

func init() {
	// The in-process Android UI talks to PortAPI on loopback 3008.
	// Set before New() so api.prep() registers this as the config default.
	// base.New overwrites any earlier SetDefaultAPIListenAddress call.
	DefaultAPIListenAddress = "127.0.0.1:3008"
}
