//go:build !android

package firewall

import (
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/spn/crew"
	"github.com/safing/portmaster/spn/sluice"
)

// Desktop path. OS redirects to sluice.
func awaitTunnelRequest(conn *network.Connection) error {
	return sluice.AwaitRequest(conn, crew.HandleSluiceRequest)
}
