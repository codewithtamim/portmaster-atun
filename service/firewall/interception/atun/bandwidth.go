//go:build android

package atun

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/log"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// bandwidthAsleep is the sleep state set by SetBandwidthSleep. It lives
// outside the worker, so a worker started while the device sleeps starts
// slow. bandwidthWake tells a running worker that the state changed.
var (
	bandwidthAsleep atomic.Bool
	bandwidthWake   = make(chan struct{}, 1)
)

// SetBandwidthSleep switches the bandwidth worker between its awake and
// asleep intervals. Safe to call at any time, from any goroutine.
func SetBandwidthSleep(enabled bool) {
	if bandwidthAsleep.Swap(enabled) == enabled {
		return
	}
	select {
	case bandwidthWake <- struct{}{}:
	default:
	}
}

// BandwidthStatsWorker reports bandwidth deltas; totals are maintained here from copy loop counts.
//
// It collects every collectInterval, or every sleepInterval while
// SetBandwidthSleep has put it to sleep. The updates carry absolute totals,
// so a slower tick batches bytes and loses none.
func BandwidthStatsWorker(ctx context.Context, collectInterval, sleepInterval time.Duration, bandwidthUpdates chan *pmpacket.BandwidthUpdate) error {
	interval := func() time.Duration {
		if bandwidthAsleep.Load() {
			return sleepInterval
		}
		return collectInterval
	}

	timer := time.NewTimer(interval())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-bandwidthWake:
			// The interval changed. Restart the wait with the new one.
		case <-timer.C:
			if !reportBandwidth(ctx, bandwidthUpdates) {
				return nil
			}
		}
		timer.Reset(interval())
	}
}

func reportBandwidth(ctx context.Context, bandwidthUpdates chan *pmpacket.BandwidthUpdate) bool {
	// A closed flow sits in the retired list only until its final counters have
	// been offered to the reporter. Reaping must therefore happen on every pass,
	// including the one where the update queue is congested - otherwise the
	// retired list grows for as long as the congestion lasts.
	queueFull := false

	for _, f := range flows.all() {
		received := f.rx.Load()
		sent := f.tx.Load()
		lastReceived := f.reportedRx.Load()
		lastSent := f.reportedTx.Load()

		if !queueFull && (received != lastReceived || sent != lastSent) {
			// Absolute totals: a dropped update is retried next tick with the
			// same running counters, so bytes are not lost.
			update := &pmpacket.BandwidthUpdate{
				ConnID:        f.id,
				BytesReceived: received,
				BytesSent:     sent,
				Method:        pmpacket.Absolute,
			}

			select {
			case bandwidthUpdates <- update:
				f.reportedRx.Store(received)
				f.reportedTx.Store(sent)
			case <-ctx.Done():
				return false
			default:
				log.Warningf("atun: bandwidth update queue is full, skipping the rest of this batch")
				queueFull = true
			}
		}

		// Best effort: a final delta that could not be pushed is dropped rather
		// than kept, as an unbounded retired list is the worse failure.
		if f.isClosed() {
			flows.removeRetired(f)
		}
	}

	return true
}
