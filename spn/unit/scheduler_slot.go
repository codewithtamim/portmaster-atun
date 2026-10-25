package unit

import (
	"math"
	"time"

	"github.com/safing/portmaster/service/mgr"
)

// SlotScheduler manages the slot and schedules units.
// Must only be started once.
func (s *Scheduler) SlotScheduler(ctx *mgr.WorkerCtx) error {
	// Start slot ticker.
	ticker := time.NewTicker(s.config.SlotDuration / 2)
	defer ticker.Stop()

	// Give clearance to all when stopping.
	defer s.clearanceUpTo.Store(math.MaxInt64 - math.MaxInt32)

	var (
		halfSlotID        uint64
		halfSlotStartedAt = time.Now()
		halfSlotEndedAt   time.Time
		halfSlotDuration  = float64(s.config.SlotDuration / 2)

		increaseStreak float64
		decreaseStreak float64
		oneStreaks     int

		cycleStatsAt = uint64(s.config.StatCycleDuration / (s.config.SlotDuration / 2)) //nolint:gosec
	)

	// waitSlot blocks until the next half-slot is due. Platforms that stop the
	// ticker while idle return false when they are done instead.
	for waitSlot(s, ticker, ctx) {
		halfSlotEndedAt = time.Now()

		switch halfSlotID % 2 {
		case 0:

			// First Half-Slot: Work Slot

			// Calculate time taken in previous slot.
			catchUpSlotDuration := halfSlotEndedAt.Sub(halfSlotStartedAt).Nanoseconds()

			// Add current slot duration to avg calculation.
			s.stats.progress.avgCatchUpSlotCnt.Add(1)
			if s.stats.progress.avgCatchUpSlotSum.Add(catchUpSlotDuration) < 0 {
				// Reset if we wrap.
				s.stats.progress.avgCatchUpSlotCnt.Store(1)
				s.stats.progress.avgCatchUpSlotSum.Store(catchUpSlotDuration)
			}

			// Reset slot counters.
			s.finished.Store(0)

			// Raise clearance according
			s.clearanceUpTo.Store(
				s.currentUnitID.Load() +
					int64(
						float64(s.slotPace.Load())*s.config.WorkSlotPercentage,
					),
			)

			// Announce start of new slot.
			s.announceNextSlot()

		default:

			// Second Half-Slot: Catch-Up Slot

			// Calculate time taken in previous slot.
			workSlotDuration := halfSlotEndedAt.Sub(halfSlotStartedAt).Nanoseconds()

			// Add current slot duration to avg calculation.
			s.stats.progress.avgWorkSlotCnt.Add(1)
			if s.stats.progress.avgWorkSlotSum.Add(workSlotDuration) < 0 {
				// Reset if we wrap.
				s.stats.progress.avgWorkSlotCnt.Store(1)
				s.stats.progress.avgWorkSlotSum.Store(workSlotDuration)
			}

			// Calculate slot duration skew correction, as slots will not run in the
			// exact specified duration.
			slotDurationSkewCorrection := halfSlotDuration / float64(workSlotDuration)

			// Calculate slot pace with performance of first half-slot.
			// Get current slot pace as float64.
			currentSlotPace := float64(s.slotPace.Load())
			// Calculate current raw slot pace.
			newRawSlotPace := float64(s.finished.Load()*2) * slotDurationSkewCorrection

			// Move slot pace in the trending direction.
			if newRawSlotPace >= currentSlotPace {
				// Adjust based on streak.
				increaseStreak++
				decreaseStreak = 0
				s.slotPace.Add(int64(
					currentSlotPace * s.config.SlotChangeRatePerStreak * increaseStreak,
				))

				// Count one-streaks.
				if increaseStreak == 1 {
					oneStreaks++
				} else {
					oneStreaks = 0
				}
			} else {
				// Adjust based on streak.
				decreaseStreak++
				increaseStreak = 0
				s.slotPace.Add(int64(
					-currentSlotPace * s.config.SlotChangeRatePerStreak * decreaseStreak,
				))

				// Enforce minimum.
				if s.slotPace.Load() < s.config.MinSlotPace {
					s.slotPace.Store(s.config.MinSlotPace)
					decreaseStreak = 0
				}

				// Count one-streaks.
				if decreaseStreak == 1 {
					oneStreaks++
				} else {
					oneStreaks = 0
				}
			}

			// Record Stats

			// Add current pace to avg calculation.
			s.stats.progress.avgPaceCnt.Add(1)
			if s.stats.progress.avgPaceSum.Add(s.slotPace.Load()) < 0 {
				// Reset if we wrap.
				s.stats.progress.avgPaceCnt.Store(1)
				s.stats.progress.avgPaceSum.Store(s.slotPace.Load())
			}

			// Check if current pace is new max.
			if s.slotPace.Load() > s.stats.progress.maxPace.Load() {
				s.stats.progress.maxPace.Store(s.slotPace.Load())
			}

			// Check if current pace is new leveled max
			if oneStreaks >= 3 && s.slotPace.Load() > s.stats.progress.maxLeveledPace.Load() {
				s.stats.progress.maxLeveledPace.Store(s.slotPace.Load())
			}
		}
		// Switch to other slot-half.
		halfSlotID++
		halfSlotStartedAt = halfSlotEndedAt

		// Cycle stats after defined time period.
		if halfSlotID%cycleStatsAt == 0 {
			s.cycleStats()
		}

		// Check if we are stopping.
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if s.stopping.IsSet() {
			return nil
		}
	}

	// Only reached when waitSlot gave up. Check why before reporting a failure.
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	if s.stopping.IsSet() {
		return nil
	}

	// We should never get here.
	// If we do, trigger a worker restart via the service worker.
	return errSchedulerEnded
}

// Stop stops the scheduler and gives clearance to all units.
func (s *Scheduler) Stop() {
	s.stopping.Set()
	wakeSlotWait(s)
}
