//go:build android

package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/safing/portmaster/service/network/netutils"
	"github.com/safing/portmaster/service/routing"
)

func TestResolverRouteHoldsLookupsOnlyUnderLockdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const global = "9.9.9.9:853"
	const local = "192.168.1.1:53"

	spnDown := routeState{useSPNLookups: true, target: routing.TargetSPN, lockdown: true}
	require.Equal(t, routeBlocked, spnDown.route(ctx, global))
	require.Equal(t, routeBlocked, spnDown.route(ctx, "[2620:fe::fe]:853"))
	require.Equal(t, routeProtected, spnDown.route(ctx, local), "local resolvers never use the SPN")
	require.Equal(t, routeProtected, spnDown.route(withLockdownExempt(ctx), global))

	noLockdown := spnDown
	noLockdown.lockdown = false
	require.Equal(t, routeProtected, noLockdown.route(ctx, global), "without lockdown lookups fall back to direct")

	noSPNLookups := spnDown
	noSPNLookups.useSPNLookups = false
	require.Equal(t, routeProtected, noSPNLookups.route(ctx, global))

	direct := spnDown
	direct.target = routing.TargetDirect
	require.Equal(t, routeProtected, direct.route(ctx, global))

	spnUp := spnDown
	spnUp.spnReady = true
	require.Equal(t, routeSPN, spnUp.route(ctx, global))

	wg := routeState{target: routing.TargetWireGuard, lockdown: true}
	require.Equal(t, routeWireGuard, wg.route(ctx, global))
}

func TestLockdownExemptDomains(t *testing.T) {
	t.Parallel()

	for _, fqdn := range []string{
		"online-check.safing.io.",
		"api.account.safing.io.",
		"API.Account.Safing.IO.",
		"updates.safing.io.",
		"safing.net.",
		"captive.apple.com.",
	} {
		require.True(t, lockdownExemptDomain(fqdn), fqdn)
	}
	for _, fqdn := range []string{
		"example.com.",
		"notsafing.io.",
		"safing.io.example.com.",
		"pm1x1.9.9.9.9.nip.io.",
	} {
		require.False(t, lockdownExemptDomain(fqdn), fqdn)
	}
}

func TestSPNDisconnectedOnlyOnReadyToNotReady(t *testing.T) {
	spnWasReady.Store(false)
	require.False(t, spnDisconnected(false))
	require.False(t, spnDisconnected(true))
	require.False(t, spnDisconnected(true))
	require.True(t, spnDisconnected(false))
	require.False(t, spnDisconnected(false))
}

func TestGlobalAddress(t *testing.T) {
	t.Parallel()

	require.True(t, globalAddress("1.1.1.1:53"))
	require.True(t, globalAddress("dns.example:443"))
	require.False(t, globalAddress("127.0.0.1:53"))
	require.False(t, globalAddress("10.0.0.1:53"))
	require.False(t, globalAddress("[fe80::1]:53"))
	require.Equal(t, netutils.Global, netutils.GetIPScope([]byte{9, 9, 9, 9}))
}

func TestSPNLockdownGate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spnDown := routeState{useSPNLookups: true, target: routing.TargetSPN, lockdown: true}
	configured := &Resolver{}
	configured.Info = &ResolverInfo{Source: ServerSourceConfigured, IPScope: netutils.Global}
	lan := &Resolver{}
	lan.Info = &ResolverInfo{Source: ServerSourceOperatingSystem, IPScope: netutils.SiteLocal}
	env := &Resolver{}
	env.Info = &ResolverInfo{Source: ServerSourceEnv, IPScope: netutils.Global}

	_, err := spnDown.gate(ctx, &Query{FQDN: "example.com."}, configured)
	require.ErrorIs(t, err, ErrSPNLockdown)

	exemptCtx, err := spnDown.gate(ctx, &Query{FQDN: "api.account.safing.io."}, configured)
	require.NoError(t, err)
	require.True(t, lockdownExempt(exemptCtx))

	for _, r := range []*Resolver{lan, env} {
		gotCtx, err := spnDown.gate(ctx, &Query{FQDN: "example.com."}, r)
		require.NoError(t, err)
		require.False(t, lockdownExempt(gotCtx))
	}

	noLockdown := spnDown
	noLockdown.lockdown = false
	_, err = noLockdown.gate(ctx, &Query{FQDN: "example.com."}, configured)
	require.NoError(t, err)
}
