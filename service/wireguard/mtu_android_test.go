//go:build android

package wireguard

import (
	"net"
	"testing"
)

func TestTunnelMTUFor(t *testing.T) {
	t.Parallel()

	up := net.FlagUp
	for _, tc := range []struct {
		name   string
		ifaces []net.Interface
		want   int
	}{
		{"no uplink", nil, maxTunnelMTU},
		{"wifi 1500", []net.Interface{{Name: "wlan0", MTU: 1500, Flags: up}}, 1420},
		{"smallest uplink wins", []net.Interface{
			{Name: "wlan0", MTU: 1500, Flags: up},
			{Name: "rmnet0", MTU: 1400, Flags: up},
		}, 1320},
		{"loopback and vpn ignored", []net.Interface{
			{Name: "lo", MTU: 65536, Flags: up | net.FlagLoopback},
			{Name: "tun0", MTU: 1280, Flags: up | net.FlagPointToPoint},
			{Name: "wlan0", MTU: 1500, Flags: up},
		}, 1420},
		{"down ignored", []net.Interface{
			{Name: "rmnet0", MTU: 1300, Flags: 0},
			{Name: "wlan0", MTU: 1500, Flags: up},
		}, 1420},
		{"tiny link clamps to minimum", []net.Interface{{Name: "rmnet0", MTU: 1280, Flags: up}}, minMTU},
		{"jumbo link clamps to maximum", []net.Interface{{Name: "eth0", MTU: 9000, Flags: up}}, maxTunnelMTU},
	} {
		if got := tunnelMTUFor(tc.ifaces); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
