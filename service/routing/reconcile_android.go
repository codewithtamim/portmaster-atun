//go:build android

package routing

import (
	"sync"

	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
)

// Reconciliation state. Remembering what we last saw is what lets reconcile tell
// "the user picked a new target" apart from "something else flipped spn/enable",
// without a re-entrancy flag, which would drop updates that arrive while it is
// set.
var (
	reconcileLock sync.Mutex
	seeded        bool
	lastTarget    Target
	lastSPN       bool
)

// reconcile makes the routing target and spn/enable agree.
//
// The target is authoritative: picking spn turns the SPN on, picking anything
// else turns it off. But spn/access flips spn/enable on its own when a
// subscription starts or expires, so a change that did not come from us is
// adopted into the target instead of being fought over. The function is
// idempotent and converges from any starting state, including the extra pass it
// triggers on itself by writing config.
func (r *Routing) reconcile() {
	reconcileLock.Lock()
	defer reconcileLock.Unlock()

	target := Current()
	spnNow := config.GetAsBool(cfgOptionEnableSPNKey, false)()

	if !seeded {
		// First pass: the target wins over whatever spn/enable happens to say.
		seeded = true
		lastTarget, lastSPN = target, spnNow
		r.applySPN(target, spnNow)
		r.EventTargetChanged.Submit(target)
		return
	}

	switch {
	case target != lastTarget:
		// The user picked a target. Drive the SPN from it.
		lastTarget = target
		r.applySPN(target, spnNow)
		log.Infof("routing: target is now %s", target)
		r.EventTargetChanged.Submit(target)

	case spnNow != lastSPN:
		// spn/enable moved on its own. Adopt it so the two cannot disagree.
		lastSPN = spnNow
		adopted := TargetDirect
		if spnNow {
			adopted = TargetSPN
		}
		if adopted == target {
			return
		}
		if err := config.SetConfigOption(CfgOptionTargetKey, string(adopted)); err != nil {
			log.Warningf("routing: failed to adopt external spn/enable change: %s", err)
			return
		}
		lastTarget = adopted
		log.Infof("routing: adopted external spn/enable change, target is now %s", adopted)
		r.EventTargetChanged.Submit(adopted)
	}
}

// applySPN writes spn/enable to match the target, if it does not already.
func (r *Routing) applySPN(target Target, spnNow bool) {
	want := target == TargetSPN
	if want == spnNow {
		lastSPN = spnNow
		return
	}
	if err := config.SetConfigOption(cfgOptionEnableSPNKey, want); err != nil {
		log.Warningf("routing: failed to set spn/enable to %v: %s", want, err)
		return
	}
	lastSPN = want
}
