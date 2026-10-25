//go:build !android

package process

import (
	"context"

	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/network/state"
)

// lookupConnectionOwner finds the PID that owns the described connection by
// searching the system network state tables.
func lookupConnectionOwner(_ context.Context, pktInfo *packet.Info, fastSearch bool) (pid int, connInbound bool, err error) {
	return state.Lookup(pktInfo, fastSearch)
}

// getProcessWithProfileByOwner returns the process that owns the described
// connection, identified by its PID.
func getProcessWithProfileByOwner(ctx context.Context, pktInfo *packet.Info) (*Process, error) {
	return GetProcessWithProfile(ctx, pktInfo.PID)
}
