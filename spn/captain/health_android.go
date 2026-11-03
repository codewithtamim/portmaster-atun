//go:build android

package captain

import "github.com/safing/portmaster/spn/docks"

// homeLinkBytesIn returns how many bytes the home hub link has received.
func homeLinkBytesIn(crane *docks.Crane) uint64 {
	if crane == nil || crane.NetState == nil {
		return 0
	}
	in, _, _, _, _, _ := crane.NetState.GetTrafficStats()
	return in
}

// homeLinkBusy reports whether a home hub ping that timed out was only stuck
// behind traffic. On a phone a busy link (a download, a video, many pages)
// can delay the ping past the health check timeout while the hub keeps
// sending data. Treating that as a dead hub marks it failing for 5 minutes and
// rebuilds every route, which breaks all SPN connections, DNS included, for
// minutes. Data that arrived during the ping proves the link is alive.
func homeLinkBusy(crane *docks.Crane, bytesBefore uint64) bool {
	return homeLinkBytesIn(crane) > bytesBefore
}
