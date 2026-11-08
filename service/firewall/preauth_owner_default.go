//go:build !android

package firewall

import "github.com/safing/portmaster/service/network"

// preAuthOwnerMatches is always true: the port check alone decides.
func preAuthOwnerMatches(*network.Connection) bool {
	return true
}
