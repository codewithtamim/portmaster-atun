//go:build android

package anet

import (
	"net"
	"net/netip"
	"testing"
)

func setNat64ForTest(t *testing.T, prefix string) {
	t.Helper()
	mu.Lock()
	old := nat64Prefix
	if prefix == "" {
		nat64Prefix = netip.Prefix{}
	} else {
		nat64Prefix = netip.MustParsePrefix(prefix)
	}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		nat64Prefix = old
		mu.Unlock()
	})
}

func TestStoreNat64KeepsOnlyTheUplinkPrefix(t *testing.T) {
	setNat64ForTest(t, "")
	storeNat64(`[
		{"name":"wlan0","nat64Prefix":"64:ff9b:1::/96"},
		{"name":"rmnet1","uplink":true,"nat64Prefix":"2001:db8:64::/96"}
	]`)
	p, ok := Nat64Prefix()
	if !ok || p.String() != "2001:db8:64::/96" {
		t.Fatalf("uplink prefix: got %v %v", p, ok)
	}

	storeNat64(`[{"name":"wlan0","uplink":true}]`)
	if _, ok := Nat64Prefix(); ok {
		t.Fatal("prefix must clear when the uplink has none")
	}

	storeNat64(`[{"name":"rmnet1","uplink":true,"nat64Prefix":"2001:db8::/64"}]`)
	if _, ok := Nat64Prefix(); ok {
		t.Fatal("only /96 prefixes are kept")
	}
}

func TestEmbeddedIPv4(t *testing.T) {
	setNat64ForTest(t, "2001:db8:64::/96")
	tests := []struct {
		ip   string
		want string
	}{
		{"64:ff9b::101:101", "1.1.1.1"},
		{"2001:db8:64::808:808", "8.8.8.8"},
		{"2001:db8:65::808:808", ""},
		{"2606:4700:4700::1111", ""},
		{"::ffff:1.1.1.1", ""},
		{"1.1.1.1", ""},
	}
	for _, tc := range tests {
		got := EmbeddedIPv4(net.ParseIP(tc.ip))
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%s: got %s, want none", tc.ip, got)
		case tc.want != "" && !got.Equal(net.ParseIP(tc.want)):
			t.Errorf("%s: got %v, want %s", tc.ip, got, tc.want)
		}
	}
}

func TestSynthesizeNat64(t *testing.T) {
	setNat64ForTest(t, "")
	if _, ok := SynthesizeNat64(net.ParseIP("1.2.3.4")); ok {
		t.Fatal("no prefix, no address")
	}
	setNat64ForTest(t, "64:ff9b::/96")
	ip, ok := SynthesizeNat64(net.ParseIP("1.2.3.4"))
	if !ok || !ip.Equal(net.ParseIP("64:ff9b::102:304")) {
		t.Fatalf("got %v %v", ip, ok)
	}
	if got := EmbeddedIPv4(ip); !got.Equal(net.ParseIP("1.2.3.4")) {
		t.Fatalf("round trip: got %v", got)
	}
	if _, ok := SynthesizeNat64(net.ParseIP("2606:4700::1")); ok {
		t.Fatal("an IPv6 address has no NAT64 form")
	}
}
