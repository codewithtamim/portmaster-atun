//go:build android

package netenv

import (
	"net"
	"time"

	"github.com/safing/portmaster/base/utils/anet"
)

var (
	monitorNetworkChangeOnlineTicker  = time.NewTicker(time.Second)
	monitorNetworkChangeOfflineTicker = time.NewTicker(time.Second)
)

type anetNotifier struct{}

func (a *anetNotifier) OnNetworkChange() {
	TriggerNetworkChangeCheck()
}

func init() {
	// On Android changes are pushed; tickers are stopped to save battery.
	monitorNetworkChangeOnlineTicker.Stop()
	monitorNetworkChangeOfflineTicker.Stop()

	anet.RegisterNotifier(&anetNotifier{})
}

func osGetInterfaceAddrs() ([]net.Addr, error) {
	return anet.InterfaceAddrs()
}

func osGetNetworkInterfaces() ([]net.Interface, error) {
	return anet.Interfaces()
}

func osGetAddrsForInterface(iface *net.Interface) ([]net.Addr, error) {
	return anet.AddrsForInterface(iface.Name)
}

// No traceroute on Android. No raw sockets. Uses timezone.
func osTracerouteSupported() bool {
	return false
}
