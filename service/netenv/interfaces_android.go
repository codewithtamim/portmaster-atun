//go:build android

package netenv

import "net"

// selectPhysicalDefaultInterfaces is not implemented on Android.
// The VPN interface is not a physical adapter, and /sys/class/net device
// entries are not a reliable signal inside an unprivileged app.
func selectPhysicalDefaultInterfaces() (*net.Interface, *net.Interface, error) {
	return nil, nil, errNoPhysicalDefaultInterface
}
