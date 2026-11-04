//go:build !android

package firewall

// startDeviceStateWatch does nothing: device policies are Android only.
func startDeviceStateWatch() {}

func stopDeviceStateWatch() {}
