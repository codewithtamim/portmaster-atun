//go:build android

package anet

import (
	"encoding/json"
	"net"
	"net/netip"
)

// wellKnownNat64 is the prefix carriers and public DNS64 services use when the
// network does not announce its own.
var wellKnownNat64 = netip.MustParsePrefix("64:ff9b::/96")

// nat64Prefix is the NAT64 prefix of the uplink, invalid while the uplink has
// none (every network that still has IPv4).
var nat64Prefix netip.Prefix

type nat64Info struct {
	Uplink      bool   `json:"uplink"`
	Nat64Prefix string `json:"nat64Prefix"`
}

// storeNat64 reads the uplink's NAT64 prefix from an interface snapshot. Only
// /96 prefixes are kept, the one length Android's 464XLAT supports.
func storeNat64(jsonStr string) {
	var list []nat64Info
	if err := json.Unmarshal([]byte(jsonStr), &list); err != nil {
		return
	}
	var found netip.Prefix
	for i := range list {
		if !list[i].Uplink || list[i].Nat64Prefix == "" {
			continue
		}
		p, err := netip.ParsePrefix(list[i].Nat64Prefix)
		if err == nil && p.Addr().Is6() && p.Bits() == 96 {
			found = p.Masked()
		}
		break
	}
	mu.Lock()
	nat64Prefix = found
	mu.Unlock()
}

// Nat64Prefix returns the NAT64 prefix of the uplink, if it has one.
func Nat64Prefix() (netip.Prefix, bool) {
	mu.RLock()
	defer mu.RUnlock()
	return nat64Prefix, nat64Prefix.IsValid()
}

// EmbeddedIPv4 returns the IPv4 address inside an IPv6 address of the uplink's
// NAT64 prefix or of the well-known prefix, and nil for every other address.
func EmbeddedIPv4(ip net.IP) net.IP {
	if len(ip) != net.IPv6len || ip.To4() != nil {
		return nil
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return nil
	}
	if wellKnownNat64.Contains(addr) {
		return net.IP(append([]byte(nil), ip[12:16]...))
	}
	if p, ok := Nat64Prefix(); ok && p.Contains(addr) {
		return net.IP(append([]byte(nil), ip[12:16]...))
	}
	return nil
}

// SynthesizeNat64 returns the address of v4 inside the uplink's NAT64 prefix.
func SynthesizeNat64(v4 net.IP) (net.IP, bool) {
	p, ok := Nat64Prefix()
	v4 = v4.To4()
	if !ok || v4 == nil {
		return nil, false
	}
	b := p.Addr().As16()
	copy(b[12:], v4)
	return net.IP(b[:]), true
}
