//go:build android

package wireguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
	"github.com/safing/portmaster/service/nameserver/nsutil"
)

var errNameserverNotListening = errors.New("wireguard: nameserver is not listening yet")

const (
	// The nameserver is not listening yet at the moment the VPN comes up, which
	// is exactly when we need it. Retry rather than failing the bring-up.
	resolveAttempts = 10
	resolveInterval = time.Second
	resolveTimeout  = 5 * time.Second
)

// lookupFn is swapped in tests.
var lookupFn = lookupViaNameserver

// resolveEndpoints resolves every peer endpoint.
//
// It succeeds as soon as one peer resolved: a profile with a dead backup peer
// must still connect. The returned map is keyed by peer index.
func resolveEndpoints(ctx context.Context, profile *Profile) (map[int]netip.AddrPort, error) {
	resolved := make(map[int]netip.AddrPort, len(profile.Peers))

	for attempt := 1; attempt <= resolveAttempts; attempt++ {
		for i, peer := range profile.Peers {
			if _, done := resolved[i]; done {
				continue
			}
			addr, err := resolveHost(ctx, peer.EndpointHost)
			if err != nil {
				log.Tracef("wireguard: resolve peer %d: %s", i, err)
				continue
			}
			resolved[i] = netip.AddrPortFrom(addr, peer.EndpointPort)
		}

		if len(resolved) == len(profile.Peers) {
			return resolved, nil
		}

		select {
		case <-ctx.Done():
			if len(resolved) > 0 {
				return resolved, nil
			}
			return nil, ctx.Err()
		case <-time.After(resolveInterval):
		}
	}

	if len(resolved) > 0 {
		// Partial is workable: the peers that did resolve can carry the tunnel.
		log.Warningf("wireguard: only %d of %d peer endpoints resolved", len(resolved), len(profile.Peers))
		return resolved, nil
	}
	return nil, ErrResolve
}

// resolveHost resolves a single endpoint host, preferring IPv4.
func resolveHost(ctx context.Context, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap(), nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	ips, err := lookupFn(lookupCtx, host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: %w", ErrResolve, err)
	}
	return preferIPv4(ips)
}

// preferIPv4 picks an address, favouring IPv4.
//
// A v6 endpoint behind DNS64/NAT64 or a v6 NAT is a common way for a tunnel to
// come up and then carry nothing.
func preferIPv4(ips []net.IP) (netip.Addr, error) {
	var fallback netip.Addr

	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if addr.Is4() {
			return addr, nil
		}
		if !fallback.IsValid() {
			fallback = addr
		}
	}

	if fallback.IsValid() {
		return fallback, nil
	}
	return netip.Addr{}, ErrResolve
}

// lookupViaNameserver resolves through Portmaster's own resolver, so the lookup
// is filtered and logged like any other.
//
// The dialer is explicitly protected: this lookup bootstraps the tunnel, so it
// must never be routed through the tunnel it is about to bring up.
func lookupViaNameserver(ctx context.Context, host string) ([]net.IP, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			addr := nsutil.BoundAddress()
			if addr == "" {
				return nil, errNameserverNotListening
			}
			d := aprotect.NewDialer()
			d.Timeout = resolveTimeout
			return d.DialContext(ctx, "udp", addr)
		},
	}

	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}
