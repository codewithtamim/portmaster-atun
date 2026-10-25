package unit

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"
)

// errSchedulerEnded is returned if the slot loop stops without the worker being cancelled.
var errSchedulerEnded = errors.New("unexpected end of scheduler")

const (
	defaultSlotDuration = 10 * time.Millisecond // 100 slots per second
	defaultMinSlotPace  = 100                   // 10 000 pps

	defaultWorkSlotPercentage      = 0.7  // 70%
	defaultSlotChangeRatePerStreak = 0.02 // 2%

	defaultStatCycleDuration = 1 * time.Minute
)

// Scheduler creates and schedules units.
// Must be created using NewScheduler().
type Scheduler struct { //nolint:maligned
	// Configuration.
	config SchedulerConfig

	// Units IDs Limit / Thresholds.

	// currentUnitID holds the last assigned Unit ID.
	currentUnitID atomic.Int64
	// clearanceUpTo holds the current threshold up to which Unit ID Units may be processed.
	clearanceUpTo atomic.Int64
	// slotPace holds the current pace. This is the base value for clearance
	// calculation, not the value of the current cleared Units itself.
	slotPace atomic.Int64
	// finished holds the amount of units that were finished within the current slot.
	finished atomic.Int64

	// Slot management.
	slotSignalA      chan struct{}
	slotSignalB      chan struct{}
	slotSignalSwitch bool
	slotSignalsLock  sync.RWMutex

	stopping     abool.AtomicBool
	unitDebugger *UnitDebugger

	// Slot-ticker parking. Only set on platforms whose waitSlot parks.
	idle *idlePark //nolint:unused // only used where waitSlot parks

	// Stats.
	stats struct {
		// Working Values.
		progress struct {
			maxPace           atomic.Int64
			maxLeveledPace    atomic.Int64
			avgPaceSum        atomic.Int64
			avgPaceCnt        atomic.Int64
			avgUnitLifeSum    atomic.Int64
			avgUnitLifeCnt    atomic.Int64
			avgWorkSlotSum    atomic.Int64
			avgWorkSlotCnt    atomic.Int64
			avgCatchUpSlotSum atomic.Int64
			avgCatchUpSlotCnt atomic.Int64
		}

		// Calculated Values.
		current struct {
			maxPace        atomic.Int64
			maxLeveledPace atomic.Int64
			avgPace        atomic.Int64
			avgUnitLife    atomic.Int64
			avgWorkSlot    atomic.Int64
			avgCatchUpSlot atomic.Int64
		}
	}
}

// idlePark is per-scheduler state for a parking slot ticker.
//
//nolint:unused // only used where waitSlot parks
type idlePark struct {
	wake     chan struct{}
	lastUnit atomic.Int64
	started  atomic.Int64
	parked   atomic.Bool
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	// SlotDuration defines the duration of one slot.
	SlotDuration time.Duration

	// MinSlotPace defines the minimum slot pace.
	// The slot pace will never fall below this value.
	MinSlotPace int64

	// WorkSlotPercentage defines the how much of a slot should be scheduled with work.
	// The remainder is for catching up and breathing room for other tasks.
	// Must be between 55% (0.55) and 95% (0.95).
	// The default value is 0.7 (70%).
	WorkSlotPercentage float64

	// SlotChangeRatePerStreak defines how many percent (0-1) the slot pace
	// should change per streak.
	// Is enforced to be able to change the minimum slot pace by at least 1.
	// The default value is 0.02 (2%).
	SlotChangeRatePerStreak float64

	// StatCycleDuration defines how often stats are calculated.
	// The default value is 1 minute.
	StatCycleDuration time.Duration
}

// NewScheduler returns a new scheduler.
func NewScheduler(config *SchedulerConfig) *Scheduler {
	// Fallback to empty config if none is given.
	if config == nil {
		config = &SchedulerConfig{}
	}

	// Create new scheduler.
	s := &Scheduler{
		config:      *config,
		slotSignalA: make(chan struct{}),
		slotSignalB: make(chan struct{}),
	}

	// Fill in defaults.
	if s.config.SlotDuration == 0 {
		s.config.SlotDuration = defaultSlotDuration
	}
	if s.config.MinSlotPace == 0 {
		s.config.MinSlotPace = defaultMinSlotPace
	}
	if s.config.WorkSlotPercentage == 0 {
		s.config.WorkSlotPercentage = defaultWorkSlotPercentage
	}
	if s.config.SlotChangeRatePerStreak == 0 {
		s.config.SlotChangeRatePerStreak = defaultSlotChangeRatePerStreak
	}
	if s.config.StatCycleDuration == 0 {
		s.config.StatCycleDuration = defaultStatCycleDuration
	}

	// Check boundaries of WorkSlotPercentage.
	switch {
	case s.config.WorkSlotPercentage < 0.55:
		s.config.WorkSlotPercentage = 0.55
	case s.config.WorkSlotPercentage > 0.95:
		s.config.WorkSlotPercentage = 0.95
	}

	// The slot change rate must be able to change the slot pace by at least 1.
	if s.config.SlotChangeRatePerStreak < (1 / float64(s.config.MinSlotPace)) {
		s.config.SlotChangeRatePerStreak = (1 / float64(s.config.MinSlotPace))

		// Debug logging:
		// fmt.Printf("--- increased SlotChangeRatePerStreak to %f\n", s.config.SlotChangeRatePerStreak)
	}

	// Initialize scheduler fields.
	initSlotWait(s)
	s.clearanceUpTo.Store(s.config.MinSlotPace)
	s.slotPace.Store(s.config.MinSlotPace)

	return s
}

func (s *Scheduler) nextSlotSignal() chan struct{} {
	s.slotSignalsLock.RLock()
	defer s.slotSignalsLock.RUnlock()

	if s.slotSignalSwitch {
		return s.slotSignalA
	}
	return s.slotSignalB
}

func (s *Scheduler) announceNextSlot() {
	s.slotSignalsLock.Lock()
	defer s.slotSignalsLock.Unlock()

	// Close new slot signal and refresh previous one.
	if s.slotSignalSwitch {
		close(s.slotSignalA)
		s.slotSignalB = make(chan struct{})
	} else {
		close(s.slotSignalB)
		s.slotSignalA = make(chan struct{})
	}

	// Switch to next slot.
	s.slotSignalSwitch = !s.slotSignalSwitch
}
