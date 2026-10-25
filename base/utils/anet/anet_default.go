//go:build !android

package anet

import "net"

func RegisterProvider(_ Provider) {}

func HasProvider() bool { return false }

func RegisterNotifier(_ Notifier) {}

func HasNotifier() bool { return false }

func TriggerNetworkChange() {}

func UpdateInterfaces(_ []NetworkInterface) {}

func UpdateInterfacesJSON(_ string) error { return nil }

func Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

func InterfaceAddrs() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}

func AddrsForInterface(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}
