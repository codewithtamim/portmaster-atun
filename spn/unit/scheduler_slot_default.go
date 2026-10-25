//go:build !android

package unit

import (
	"time"

	"github.com/safing/portmaster/service/mgr"
)

// waitSlot blocks until the next half-slot is due.
// The ticker runs for as long as the scheduler does, so this only returns
// false if the ticker channel is closed, which it never is.
func waitSlot(_ *Scheduler, ticker *time.Ticker, _ *mgr.WorkerCtx) bool {
	_, ok := <-ticker.C

	return ok
}

// wakeSlotWait does nothing here, as waitSlot only ever waits on the ticker.
func wakeSlotWait(_ *Scheduler) {}

// initSlotWait does nothing here, as waitSlot never parks.
func initSlotWait(_ *Scheduler) {}

// noteUnit does nothing here, as waitSlot never parks.
func noteUnit(_ *Scheduler) {}
