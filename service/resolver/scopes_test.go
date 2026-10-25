package resolver

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestDocumentationNamesUseGlobalResolversOnlyOnAndroid(t *testing.T) { //nolint:paralleltest // mutates resolver lists
	global := &Resolver{
		Info: &ResolverInfo{
			Name:   "Configured",
			Type:   ServerTypeDoT,
			Source: ServerSourceConfigured,
			IP:     net.ParseIP("1.1.1.2"),
			Domain: "cloudflare-dns.com",
			Port:   853,
		},
	}
	local := &Resolver{
		Info: &ResolverInfo{
			Name:   "Local",
			Type:   ServerTypeDoT,
			Source: ServerSourceConfigured,
			IP:     net.ParseIP("192.168.1.1"),
			Domain: "lan.example",
			Port:   853,
		},
	}

	resolversLock.Lock()
	savedGlobal, savedLocal := globalResolvers, localResolvers
	globalResolvers = []*Resolver{global}
	localResolvers = []*Resolver{local}
	resolversLock.Unlock()
	t.Cleanup(func() {
		resolversLock.Lock()
		globalResolvers = savedGlobal
		localResolvers = savedLocal
		resolversLock.Unlock()
	})

	for _, name := range []string{"example.com.", "www.example.net.", "example.org."} {
		q := &Query{FQDN: name, QType: dns.Type(dns.TypeA)}
		if !q.check() {
			t.Fatalf("query %s failed check", name)
		}
		selected, source, _ := GetResolversInScope(context.Background(), q)
		wantGlobal := documentationNamesResolvedGlobally(q.dotPrefixedFQDN)
		if wantGlobal {
			if source != ServerSourceConfigured || len(selected) != 1 || selected[0] != global {
				t.Fatalf("%s: got source %q resolvers %v, want configured global", name, source, selected)
			}
			continue
		}
		if source != "special" || len(selected) != 1 || selected[0] != local {
			t.Fatalf("%s: got source %q resolvers %v, want local special", name, source, selected)
		}
	}

	home := &Query{FQDN: "printer.home.", QType: dns.Type(dns.TypeA)}
	if !home.check() {
		t.Fatal("home query failed check")
	}
	selected, source, _ := GetResolversInScope(context.Background(), home)
	if source != "special" || len(selected) != 1 || selected[0] != local {
		t.Fatalf("home: got source %q resolvers %v, want local special on every platform", source, selected)
	}
}
