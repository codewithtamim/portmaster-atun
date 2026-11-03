//go:build android

package atun

import (
	"net"
	"testing"
)

func TestLocalDNSHosts(t *testing.T) {
	t.Cleanup(func() { SetLocalDNSHosts(nil) })

	SetLocalDNSHosts([]string{"192.168.1.1", "fd00::1", "not-an-ip"})
	if !isLocalDNSHost(net.ParseIP("192.168.1.1")) {
		t.Fatal("router address not matched")
	}
	if !isLocalDNSHost(net.ParseIP("::ffff:192.168.1.1")) {
		t.Fatal("v4-mapped router address not matched")
	}
	if !isLocalDNSHost(net.ParseIP("fd00::1")) {
		t.Fatal("v6 router address not matched")
	}
	if isLocalDNSHost(net.ParseIP("192.168.1.2")) {
		t.Fatal("other local address matched")
	}
	SetLocalDNSHosts(nil)
	if isLocalDNSHost(net.ParseIP("192.168.1.1")) {
		t.Fatal("cleared set still matched")
	}
}
