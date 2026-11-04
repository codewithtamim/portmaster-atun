//go:build android

package unit

import (
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/service/mgr"
)

// idleParkAfter is how long to wait with no new units before stopping the slot
// ticker. A phone spends most of its time with nothing to schedule, and a
// ticker that keeps firing there costs wakeups and battery for no work.
//
// The ticker fires every half slot (5 ms, 200 times a second). With the screen
// off the SPN still sees a trickle of units (gossip, keepalives, route
// attempts), and a full second of ticking per unit kept it running nearly all
// the time: about 28% of a core while idle. Parking is only allowed once no
// unit is waiting for clearance (see shouldPark), so a short window is safe.
const idleParkAfter = 100 * time.Millisecond

// slotWaitState is the per-scheduler state of the parking slot ticker.
type slotWaitState struct {
	wake     chan struct{}
	lastUnit atomic.Int64
	started  atomic.Int64
	parked   atomic.Bool
}

// initSlotWait sets up parking state. Called once from NewScheduler, so
// every later reader sees the wake channel without synchronising.
func initSlotWait(s *Scheduler) {
	s.idle.wake = make(chan struct{}, 1)
	s.idle.started.Store(time.Now().UnixNano())
}

// noteUnit records unit activity and wakes a parked ticker.
func noteUnit(s *Scheduler) {
	s.idle.lastUnit.Store(time.Now().UnixNano())
	wakeSlotWait(s)
}

// wakeSlotWait releases a parked ticker.
func wakeSlotWait(s *Scheduler) {
	if s.idle.parked.CompareAndSwap(true, false) {
		select {
		case s.idle.wake <- struct{}{}:
		default:
		}
	}
}

func shouldPark(s *Scheduler, st *slotWaitState) bool {
	if s.stopping.IsSet() {
		return false
	}
	// A unit still waiting in WaitForSlot needs the slots to advance.
	if s.clearanceUpTo.Load() < s.currentUnitID.Load() {
		return false
	}
	last := st.lastUnit.Load()
	if last == 0 {
		last = st.started.Load()
	}

	return time.Since(time.Unix(0, last)) >= idleParkAfter
}

// waitSlot blocks until the next half-slot is due, stopping the ticker
// entirely while there is nothing to schedule.
//
// After a park, slotStartedAt is moved to the wake-up time, so the first slot
// does not count the parked time as its own duration. Counting it would skew
// the slot stats and drop the pace to its minimum after every park.
//
// When the worker is cancelled while parked, it returns true without waiting
// for the ticker, so the scheduler loop sees the cancellation and ends cleanly.
func waitSlot(s *Scheduler, ticker *time.Ticker, ctx *mgr.WorkerCtx, slotStartedAt *time.Time) bool {
	st := &s.idle

	// Re-check after claiming the park: a unit may have arrived in between,
	// and parking then would sleep through it.
	if shouldPark(s, st) && st.parked.CompareAndSwap(false, true) {
		if shouldPark(s, st) {
			ticker.Stop()
			select {
			case <-st.wake:
				ticker.Reset(s.config.SlotDuration / 2)
				*slotStartedAt = time.Now()
			case <-ctx.Done():
				return true
			}
		} else {
			st.parked.Store(false)
		}
	}

	_, ok := <-ticker.C

	return ok
}
