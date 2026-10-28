//go:build android

package interception

import (
	"time"

	"github.com/safing/portmaster/service/firewall/interception/atun"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
)

func startInterception(packets chan packet.Packet) error {
	err := StartAtunInterception(packets)
	if err != nil {
		return err
	}

	// There is no eBPF on Android. The forwarders report connections and
	// this worker reports their byte counts. While the device sleeps it
	// collects rarely: every collection with traffic writes to the history
	// database. The app drives that through atun.SetBandwidthSleep.
	module.mgr.Go("atun bandwidth stats monitor", func(wc *mgr.WorkerCtx) error {
		return atun.BandwidthStatsWorker(wc.Ctx(), 1*time.Second, 30*time.Second, BandwidthUpdates)
	})

	return nil
}

func stopInterception() error {
	return StopAtunInterception()
}

// ResetVerdictOfAllConnections closes every flow that has a verdict, so apps
// reconnect and are decided again under the new rules.
func ResetVerdictOfAllConnections() error {
	return atun.DeleteAllMarkedConnection()
}

// UpdateVerdictOfConnection closes the flow of conn, so its next connection
// is decided again. A verdict cannot change on a flow that is already proxied.
func UpdateVerdictOfConnection(conn *network.Connection) error {
	return atun.DeleteMarkedConnection(conn)
}
