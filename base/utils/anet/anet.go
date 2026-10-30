// Package anet carries the network interface details reported by the Android
// app, since the core cannot read them from the system itself.
package anet

import (
	"errors"
	"fmt"
	"net"
)

var (
	errParseIP          = errors.New("failed to parse ip")
	errIPv6FlagMismatch = errors.New("isIPv6 flag mismatch")
	errInvalidPrefix    = errors.New("invalid prefix length")
)

// NetworkAddress is an interface address with its prefix length.
type NetworkAddress struct {
	Addr         string `json:"addr"`
	PrefixLength int    `json:"prefixLength"`
	IsIPv6       bool   `json:"isIPv6"`
}

// ToIPNet returns the address as a net.IPNet. It fails if the address does not
// parse, if IsIPv6 does not match the address family, or if the prefix length
// is out of range.
func (a *NetworkAddress) ToIPNet() (*net.IPNet, error) {
	ip := net.ParseIP(a.Addr)
	if ip == nil {
		return nil, fmt.Errorf("%w: %s", errParseIP, a.Addr)
	}
	// Normalize IPv4 to 4-byte to match mask length.
	if ip4 := ip.To4(); ip4 != nil {
		if a.IsIPv6 {
			return nil, fmt.Errorf("%w: %s", errIPv6FlagMismatch, a.Addr)
		}
		ip = ip4
	} else if !a.IsIPv6 {
		return nil, fmt.Errorf("%w: %s", errIPv6FlagMismatch, a.Addr)
	}
	maskLen := net.IPv4len * 8
	if a.IsIPv6 {
		maskLen = net.IPv6len * 8
	}
	mask := net.CIDRMask(a.PrefixLength, maskLen)
	if mask == nil {
		return nil, fmt.Errorf("%w: %d for %s", errInvalidPrefix, a.PrefixLength, a.Addr)
	}
	return &net.IPNet{IP: ip, Mask: mask}, nil
}

// String returns the address without its prefix length.
func (a *NetworkAddress) String() string {
	return a.Addr
}

// NetworkInterface is an interface as reported by the Android app.
type NetworkInterface struct {
	Name      string           `json:"name"`
	Index     int              `json:"index"`
	MTU       int              `json:"mtu"`
	Up        bool             `json:"up"`
	Multicast bool             `json:"multicast"`
	Loopback  bool             `json:"loopback"`
	P2P       bool             `json:"p2p"`
	Addresses []NetworkAddress `json:"addresses"`

	Flags net.Flags `json:"-"`
}

// Provider returns the current interfaces as a JSON encoded []NetworkInterface.
type Provider interface {
	GetInterfacesJSON() string
}

// Notifier is told when the network changes.
type Notifier interface {
	OnNetworkChange()
}

// ToNetInterface returns the interface as a net.Interface and its addresses.
// Addresses that fail to parse are skipped.
func (i *NetworkInterface) ToNetInterface() (net.Interface, []net.Addr) {
	i.setFlagsValue()
	iface := net.Interface{
		Index:        i.Index,
		MTU:          i.MTU,
		Name:         i.Name,
		Flags:        i.Flags,
		HardwareAddr: nil,
	}
	var addrs []net.Addr
	for _, a := range i.Addresses {
		ipNet, err := a.ToIPNet()
		if err != nil {
			continue
		}
		addrs = append(addrs, ipNet)
	}
	return iface, addrs
}

func (i *NetworkInterface) setFlagsValue() {
	i.Flags = 0
	if i.Up {
		i.Flags |= net.FlagUp
	}
	if i.Loopback {
		i.Flags |= net.FlagLoopback
	}
	if i.P2P {
		i.Flags |= net.FlagPointToPoint
	}
	if i.Multicast {
		i.Flags |= net.FlagMulticast
	}
}
