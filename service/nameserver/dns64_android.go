//go:build android

package nameserver

import (
	"context"
	"net"

	"github.com/miekg/dns"

	"github.com/safing/portmaster/base/utils/anet"
	"github.com/safing/portmaster/service/firewall"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/netutils"
	"github.com/safing/portmaster/service/resolver"
)

// synthesizeAAAA answers an AAAA query for a name that only has IPv4 addresses.
//
// On a network without IPv4, the carrier's resolver does this itself. Our
// resolver asks a public server instead, which cannot know the carrier's NAT64
// prefix, so an app that only speaks IPv6 would find no address at all. The
// answer is built after the filter, from IPv4 addresses that passed it, and is
// never cached because the prefix belongs to the current network.
func synthesizeAAAA(ctx context.Context, conn *network.Connection, q *resolver.Query, rrCache *resolver.RRCache) *resolver.RRCache {
	if rrCache == nil || q.QType != dns.Type(dns.TypeAAAA) || rrCache.RCode != dns.RcodeSuccess {
		return rrCache
	}
	if _, ok := anet.Nat64Prefix(); !ok {
		return rrCache
	}
	if rrCache.Resolver != nil && rrCache.Resolver.Type == resolver.ServerTypeEnv {
		return rrCache
	}
	for _, rr := range rrCache.Answer {
		if _, ok := rr.(*dns.AAAA); ok {
			return rrCache
		}
	}

	aQuery := *q
	aQuery.QType = dns.Type(dns.TypeA)
	aCache, err := resolver.Resolve(ctx, &aQuery)
	if err != nil || aCache == nil || aCache.RCode != dns.RcodeSuccess {
		return rrCache
	}
	aCache = firewall.FilterResolvedDNS(ctx, conn, &aQuery, aCache)
	if aCache == nil || conn.Verdict == network.VerdictBlock || conn.Verdict == network.VerdictDrop {
		return rrCache
	}

	answer, ok := nat64Answer(aCache.Answer, anet.SynthesizeNat64)
	if !ok {
		return rrCache
	}
	out := rrCache.ShallowCopy()
	out.Answer = answer
	out.Ns = nil
	out.Expires = aCache.Expires
	out.Filtered = aCache.Filtered
	out.FilteredEntries = aCache.FilteredEntries
	return out
}

// nat64Answer turns the public IPv4 addresses of an A answer into AAAA records
// and keeps its CNAME chain. It reports false when no address was left.
func nat64Answer(answer []dns.RR, synth func(net.IP) (net.IP, bool)) ([]dns.RR, bool) {
	var out []dns.RR
	found := false
	for _, rr := range answer {
		switch v := rr.(type) {
		case *dns.CNAME:
			out = append(out, dns.Copy(v))
		case *dns.A:
			if !netutils.GetIPScope(v.A).IsGlobal() {
				continue
			}
			ip, ok := synth(v.A)
			if !ok {
				continue
			}
			out = append(out, &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   v.Hdr.Name,
					Rrtype: dns.TypeAAAA,
					Class:  v.Hdr.Class,
					Ttl:    v.Hdr.Ttl,
				},
				AAAA: ip,
			})
			found = true
		}
	}
	return out, found
}
