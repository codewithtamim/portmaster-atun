//go:build android

package firewall

import (
	"testing"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
)

func TestReevaluateEnded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		conn *network.Connection
		want bool
	}{
		{"nil", nil, false},
		{"icmp echo", &network.Connection{Type: network.IPConnection, IPProtocol: packet.ICMP}, true},
		{"icmpv6 echo", &network.Connection{Type: network.IPConnection, IPProtocol: packet.ICMPv6}, true},
		{"tcp", &network.Connection{Type: network.IPConnection, IPProtocol: packet.TCP}, false},
		{"udp", &network.Connection{Type: network.IPConnection, IPProtocol: packet.UDP}, false},
		{"dns request", &network.Connection{Type: network.DNSRequest, IPProtocol: packet.ICMP}, false},
	}
	for _, tc := range tests {
		if got := reevaluateEnded(tc.conn); got != tc.want {
			t.Errorf("%s: reevaluateEnded = %v, want %v", tc.name, got, tc.want)
		}
	}
}
