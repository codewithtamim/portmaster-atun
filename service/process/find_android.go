//go:build android

package process

import (
	"context"
	"fmt"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/network/packet"
)

// lookupConnectionOwner finds the UID that owns the described connection by
// asking ConnectivityManager.getConnectionOwnerUid through the Android side.
//
// The returned PID is shaped for the shared firewall: our own UID maps to
// os.Getpid() so Pid == ownPID still grants Portmaster's sockets. See
// pidForUID for the other apps.
func lookupConnectionOwner(ctx context.Context, pktInfo *packet.Info, _ bool) (pid int, connInbound bool, err error) {
	pktInfo.UID = nil

	uid, err := aconn.LookupUID(
		uint8(pktInfo.Protocol),
		pktInfo.LocalIP(), pktInfo.LocalPort(),
		pktInfo.RemoteIP(), pktInfo.RemotePort(),
	)
	if err != nil {
		return UndefinedProcessID, pktInfo.Inbound, fmt.Errorf("failed to look up connection owner: %w", err)
	}

	pktInfo.UID = &uid
	pid = pidForUID(uid)
	log.Tracer(ctx).Tracef("process: got uid %d (pid %d) from connection owner lookup", uid, pid)

	return pid, pktInfo.Inbound, nil
}

// getProcessWithProfileByOwner returns the app that owns the described
// connection, identified by its UID.
func getProcessWithProfileByOwner(ctx context.Context, pktInfo *packet.Info) (*Process, error) {
	if pktInfo.UID == nil {
		return GetProcessWithProfileByUID(ctx, UndefinedProcessID)
	}
	return GetProcessWithProfileByUID(ctx, *pktInfo.UID)
}
