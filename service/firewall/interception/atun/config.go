//go:build android

package atun

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"
)

// TunConfig is the TUN interface VpnService established. StartTun stores it
// with SetTunConfig, and Open builds the next session from it.
//
// It is held in memory, not in the process environment: setenv is not safe
// against concurrent getenv in native code sharing the process, and a value
// that is parsed once here cannot fail later inside Open.
type TunConfig struct {
	// FD is borrowed. Java keeps the ParcelFileDescriptor and closes it; Open
	// dups it and Go only ever closes its dup. See the dup model in stack.go.
	FD int
	// MTU of the TUN, as set with Builder.setMtu.
	MTU uint32
	// Addr is the IPv4 from Builder.addAddress. The gVisor NIC must hold it,
	// or locally answered packets (ICMP, DNS replies) have no source.
	Addr netip.Addr
	// FakeDNS is the IPv4 from Builder.addDnsServer. Nothing listens there;
	// atun answers for it.
	FakeDNS netip.Addr
}

// NewTunConfig validates the VpnService parameters. An mtu of 0 and empty
// addresses take the defaults that match the app's VpnService.Builder.
func NewTunConfig(fd, mtu int, tunAddr, fakeDNS string) (TunConfig, error) {
	if fd < 0 {
		return TunConfig{}, fmt.Errorf("%w: %d", ErrInvalidFD, fd)
	}
	if mtu < 0 || mtu > 65535 {
		return TunConfig{}, fmt.Errorf("%w: %d", ErrInvalidMTU, mtu)
	}
	if mtu == 0 {
		mtu = defaultMTU
	}

	addr, err := parseIPv4Addr(orDefault(tunAddr, defaultTunAddr), ErrInvalidTunAddr)
	if err != nil {
		return TunConfig{}, err
	}
	dns, err := parseIPv4Addr(orDefault(fakeDNS, defaultFakeDNSAddr), ErrInvalidDNSAddr)
	if err != nil {
		return TunConfig{}, err
	}

	return TunConfig{
		FD:      fd,
		MTU:     uint32(mtu),
		Addr:    addr,
		FakeDNS: dns,
	}, nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseIPv4Addr(raw string, invalid error) (netip.Addr, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w %q: %w", invalid, raw, err)
	}
	addr = addr.Unmap()
	if !addr.Is4() {
		return netip.Addr{}, fmt.Errorf("%w %q: not IPv4", invalid, raw)
	}
	return addr, nil
}

var (
	tunConfigLock sync.Mutex
	tunConfig     *TunConfig
)

// SetTunConfig stores the TUN the next Open builds on.
func SetTunConfig(cfg TunConfig) {
	tunConfigLock.Lock()
	defer tunConfigLock.Unlock()
	tunConfig = &cfg
}

// ClearTunConfig forgets the TUN, so a later Open cannot dup an fd number
// that has since been closed and reused for something else.
func ClearTunConfig() {
	tunConfigLock.Lock()
	defer tunConfigLock.Unlock()
	tunConfig = nil
}

// CurrentTunConfig returns the stored TUN, if any.
func CurrentTunConfig() (TunConfig, bool) {
	tunConfigLock.Lock()
	defer tunConfigLock.Unlock()
	if tunConfig == nil {
		return TunConfig{}, false
	}
	return *tunConfig, true
}
