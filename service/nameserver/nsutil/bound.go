package nsutil

import (
	"strings"
	"sync/atomic"
)

var boundAddress atomic.Pointer[string]

// SetBoundAddress records the address the nameserver actually listens on.
// On Android the configured listen address uses port 0, which is not a dial
// target, so in-process clients must dial this address instead.
// Addresses with port 0 are ignored.
func SetBoundAddress(addr string) {
	if addr == "" || strings.HasSuffix(addr, ":0") {
		return
	}
	boundAddress.Store(&addr)
}

// BoundAddress returns the address set by SetBoundAddress, or "" if the
// nameserver is not listening yet.
func BoundAddress() string {
	if addr := boundAddress.Load(); addr != nil {
		return *addr
	}
	return ""
}
