//go:build android

package atun

import (
	"context"
	"time"

	"github.com/safing/portmaster/base/log"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// BandwidthStatsWorker reports bandwidth deltas; totals are maintained here from copy loop counts.
func BandwidthStatsWorker(ctx context.Context, collectInterval time.Duration, bandwidthUpdates chan *pmpacket.BandwidthUpdate) error {
	ticker := time.NewTicker(collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !reportBandwidth(ctx, bandwidthUpdates) {
				return nil
			}
		}
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
