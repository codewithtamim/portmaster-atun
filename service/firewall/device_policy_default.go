//go:build !android

package firewall

import (
	"context"

	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/profile"
)

func checkDevicePolicy(context.Context, *network.Connection, *profile.LayeredProfile, packet.Packet) bool {
	return false
}
