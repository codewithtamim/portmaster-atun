//go:build android

package access

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/spn/conf"
)

// tunnelUp is whether the VPN tunnel is running. Without it no app traffic
// reaches the engine, so an SPN connection would only cost battery: hub
// gossip, health checks and keepalives with nothing to route.
var tunnelUp atomic.Bool

// spnWanted reports whether the SPN should run: enabled by the user and
// needed, because protection is on.
func spnWanted(enabled bool) bool {
	return enabled && tunnelUp.Load()
}

// tunnelDownGrace is how long the tunnel must stay down before the SPN stops.
// The app rebuilds the tunnel on network changes with StopTun then StartTun;
// stopping the SPN for that gap would drop every SPN connection and pay for a
// new connection to the home hub each time.
const tunnelDownGrace = 30 * time.Second

var (
	stopTimerLock sync.Mutex
	stopTimer     *time.Timer
)

// SetTunnelUp records whether the VPN tunnel runs and starts or stops the SPN
// to match. The spn/enable setting is left alone: it is the user's routing
// choice and comes back into effect when protection is turned on again.
func SetTunnelUp(up bool) {
	// Swap under the lock, so overlapping calls apply in the order they set
	// the state and the grace timer always matches the last one.
	stopTimerLock.Lock()
	defer stopTimerLock.Unlock()
	if tunnelUp.Swap(up) == up {
		return
	}

	if stopTimer != nil {
		stopTimer.Stop()
		stopTimer = nil
	}

	if up {
		applySPNState()
		return
	}
	stopTimer = time.AfterFunc(tunnelDownGrace, applySPNState)
}

// applySPNState starts or stops the SPN group to match spnWanted.
func applySPNState() {
	if module == nil || !conf.Integrated() || module.instance.IsShuttingDown() {
		return
	}
	if spnWanted(config.GetAsBool("spn/enable", false)()) {
		log.Info("spn: tunnel is up, starting SPN")
		module.mgr.Go("ensure SPN is started", module.instance.SPNGroup().EnsureStartedWorker)
	} else {
		log.Info("spn: tunnel is down, stopping SPN")
		module.mgr.Go("ensure SPN is stopped", module.instance.SPNGroup().EnsureStoppedWorker)
	}
}
