//go:build android

package wireguard

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestParseBootstrapServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		ok   bool
		want bootstrapServer
	}{
		{
			url:  "dot://cloudflare-dns.com?ip=1.1.1.2&name=Cloudflare&blockedif=zeroip",
			ok:   true,
			want: bootstrapServer{kind: "tls", addr: netip.MustParseAddrPort("1.1.1.2:853"), serverName: "cloudflare-dns.com"},
		},
		{
			url:  "tls://9.9.9.9:8853?verify=dns.quad9.net",
			ok:   true,
			want: bootstrapServer{kind: "tls", addr: netip.MustParseAddrPort("9.9.9.9:8853"), serverName: "dns.quad9.net"},
		},
		{
			url:  "doh://dns.example.net/custom?ip=2001:db8::1",
			ok:   true,
			want: bootstrapServer{kind: "https", addr: netip.MustParseAddrPort("[2001:db8::1]:443"), serverName: "dns.example.net", path: "/custom"},
		},
		{
			url:  "https://[2001:db8::53]?verify=dns.example.net",
			ok:   true,
			want: bootstrapServer{kind: "https", addr: netip.MustParseAddrPort("[2001:db8::53]:443"), serverName: "dns.example.net"},
		},
		{
			url:  "dns://192.0.2.53",
			ok:   true,
			want: bootstrapServer{kind: "udp", addr: netip.MustParseAddrPort("192.0.2.53:53")},
		},
		{
			url:  "tcp://192.0.2.53:5353",
			ok:   true,
			want: bootstrapServer{kind: "tcp", addr: netip.MustParseAddrPort("192.0.2.53:5353")},
		},
		// A server known only by name would need a lookup of its own.
		{url: "dot://dns.example.net", ok: false},
		{url: "doh://dns.example.net/dns-query", ok: false},
		{url: "dns://dns.example.net", ok: false},
		// No name to check the certificate against.
		{url: "dot://192.0.2.1", ok: false},
		{url: "ftp://192.0.2.1", ok: false},
		{url: "dns://192.0.2.1:0", ok: false},
		{url: "dns://192.0.2.1:99999", ok: false},
		{url: "://", ok: false},
	}

	for _, tc := range tests {
		got, ok := parseBootstrapServer(tc.url)
		if ok != tc.ok {
			t.Errorf("%s: ok = %t, want %t", tc.url, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.url, got, tc.want)
		}
	}
}

func TestBootstrapServersKeepsOrder(t *testing.T) {
	t.Parallel()

	got := bootstrapServers([]string{
		"dot://dns.example.net",
		"dot://cloudflare-dns.com?ip=1.1.1.2",
		"dns://192.0.2.53",
	})
	if len(got) != 2 || got[0].kind != "tls" || got[1].kind != "udp" {
		t.Errorf("bootstrapServers = %+v, want the DoT then the plain server", got)
	}
}

// Not parallel: swaps package lookups.
func TestLookupEndpointOrder(t *testing.T) {
	origProtected, origNameserver := protectedLookupFn, nameserverLookup
	t.Cleanup(func() { protectedLookupFn, nameserverLookup = origProtected, origNameserver })

	protectedIP := net.ParseIP("192.0.2.1")
	nameserverIP := net.ParseIP("192.0.2.2")
	var protectedCalls, nameserverCalls int
	protectedErr := error(nil)
	protectedLookupFn = func(context.Context, string) ([]net.IP, error) {
		protectedCalls++
		if protectedErr != nil {
			return nil, protectedErr
		}
		return []net.IP{protectedIP}, nil
	}
	nameserverLookup = func(context.Context, string) ([]net.IP, error) {
		nameserverCalls++
		return []net.IP{nameserverIP}, nil
	}

	// The protected path answers: the nameserver, which may be blocked
	// until the tunnel is up, is not asked.
	ips, err := lookupEndpoint(t.Context(), "vpn.example.com")
	if err != nil || len(ips) != 1 || !ips[0].Equal(protectedIP) || nameserverCalls != 0 {
		t.Fatalf("protected answer: ips=%v err=%v nameserver calls=%d", ips, err, nameserverCalls)
	}

	// The protected path fails: fall back to the nameserver.
	protectedErr = errNoBootstrapServer
	ips, err = lookupEndpoint(t.Context(), "vpn.example.com")
	if err != nil || len(ips) != 1 || !ips[0].Equal(nameserverIP) || nameserverCalls != 1 {
		t.Fatalf("fallback: ips=%v err=%v nameserver calls=%d", ips, err, nameserverCalls)
	}

	// Both fail: the error names both.
	nameserverLookup = func(context.Context, string) ([]net.IP, error) {
		return nil, errNameserverNotListening
	}
	_, err = lookupEndpoint(t.Context(), "vpn.example.com")
	if !errors.Is(err, errNoBootstrapServer) || !errors.Is(err, errNameserverNotListening) {
		t.Fatalf("both failing: err = %v", err)
	}
	if protectedCalls != 3 {
		t.Errorf("protected lookup ran %d times, want 3", protectedCalls)
	}
}

// Not parallel: swaps package lookups.
func TestLookupEndpointFallbackAfterHang(t *testing.T) {
	origProtected, origNameserver := protectedLookupFn, nameserverLookup
	t.Cleanup(func() { protectedLookupFn, nameserverLookup = origProtected, origNameserver })

	// The network drops the protected queries, so that lookup runs until its
	// context ends. The nameserver must still get a live context.
	protectedLookupFn = func(ctx context.Context, _ string) ([]net.IP, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cached := net.ParseIP("192.0.2.3")
	nameserverLookup = func(ctx context.Context, _ string) ([]net.IP, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return []net.IP{cached}, nil
	}

	ctx, cancel := context.WithTimeout(t.Context(), resolveTimeout)
	defer cancel()
	ips, err := lookupEndpoint(ctx, "vpn.example.com")
	if err != nil || len(ips) != 1 || !ips[0].Equal(cached) {
		t.Fatalf("ips=%v err=%v, want the nameserver's answer", ips, err)
	}
}

func TestLookupProtectedWithoutServers(t *testing.T) {
	t.Parallel()

	// The nameserver option is not registered in tests, so there is nothing
	// to ask. That must be an error, not a hang.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := lookupProtected(ctx, "vpn.example.com"); err == nil {
		t.Error("lookupProtected succeeded with no servers")
	}
}

// TestQueryServerLive asks real public servers over each transport. It needs
// a network, so it runs only with WG_LIVE_DNS=1.
func TestQueryServerLive(t *testing.T) {
	if os.Getenv("WG_LIVE_DNS") != "1" {
		t.Skip("set WG_LIVE_DNS=1 to run")
	}
	t.Parallel()

	for _, raw := range []string{
		"dot://cloudflare-dns.com?ip=1.1.1.1",
		"doh://cloudflare-dns.com/dns-query?ip=1.1.1.1",
		"dns://9.9.9.9",
		"tcp://9.9.9.9",
	} {
		server, ok := parseBootstrapServer(raw)
		if !ok {
			t.Fatalf("%s did not parse", raw)
		}
		ips, err := queryServer(t.Context(), server, "one.one.one.one")
		if err != nil || len(ips) == 0 {
			t.Errorf("%s: ips=%v err=%v", raw, ips, err)
			continue
		}
		t.Logf("%s: %v", raw, ips)
	}
}
