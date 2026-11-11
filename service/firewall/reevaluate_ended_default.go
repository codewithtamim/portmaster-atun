//go:build !android

package firewall

import "github.com/safing/portmaster/service/network"

// reevaluateEnded keeps the existing behaviour outside Android: a connection
// that has ended is not decided again.
func reevaluateEnded(*network.Connection) bool {
	return false
}
