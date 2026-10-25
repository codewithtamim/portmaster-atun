//go:build android

package state

import (
	"github.com/safing/portmaster/service/network/socket"
)

// The system state tables are not available on Android: /proc/net is not
// readable by an unprivileged app. Connection ownership is resolved through
// base/utils/aconn instead, see service/process/find_android.go.

func getTCP4Table() (connections []*socket.ConnectionInfo, listeners []*socket.BindInfo, err error) {
	return nil, nil, nil
}

func getTCP6Table() (connections []*socket.ConnectionInfo, listeners []*socket.BindInfo, err error) {
	return nil, nil, nil
}

func getUDP4Table() (binds []*socket.BindInfo, err error) {
	return nil, nil
}

func getUDP6Table() (binds []*socket.BindInfo, err error) {
	return nil, nil
}

// CheckPID checks the if socket info already has a PID and if not, tries to find it.
// Depending on the OS, this might be a no-op.
func CheckPID(socketInfo socket.Info, connInbound bool) (pid int, inbound bool, err error) {
	return socketInfo.GetPID(), connInbound, nil
}
