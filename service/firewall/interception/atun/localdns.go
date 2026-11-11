//go:build android

package atun

import (
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/safing/portmaster/base/log"
)

// localDNSHosts are the current network's own DNS servers and default
// gateway on private addresses, usually the home router. Local traffic stays
// outside the tunnel, except that the VpnService routes these addresses in so
// their port 53 can be filtered like any other resolver. Everything else sent to them
// (the router's admin page, UPnP, casting helpers) is local traffic and is
// passed through without a firewall decision, as if it never entered the
// tunnel.
var localDNSHosts atomic.Pointer[map[netip.Addr]struct{}]

// SetLocalDNSHosts replaces the set of routed-in local DNS servers.
// Unparsable entries are skipped.
func SetLocalDNSHosts(hosts []string) {
	set := make(map[netip.Addr]struct{}, len(hosts))
	for _, h := range hosts {
		addr, err := netip.ParseAddr(h)
		if err != nil {
			log.Warningf("atun: ignoring local DNS host %q: %s", h, err)
			continue
		}
		set[addr.Unmap()] = struct{}{}
	}
	localDNSHosts.Store(&set)
}

// isLocalDNSHost reports whether ip is one of the routed-in local DNS servers.
func isLocalDNSHost(ip net.IP) bool {
	set := localDNSHosts.Load()
	if set == nil || len(*set) == 0 {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	_, found := (*set)[addr.Unmap()]
	return found
}
