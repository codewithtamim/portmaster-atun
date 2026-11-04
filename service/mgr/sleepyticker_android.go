//go:build android

package mgr

import "time"

// setSleepPlatform sets the sleep mode the way SetSleep describes. Android
// enters and leaves sleep with the screen, so both of these happen:
//   - Waking a ticker that ticks during sleep must not close its sleep
//     channel, which it never created.
//   - Enabling sleep twice must not replace a sleep channel that Wait is
//     already blocked on.
//
// A sleepDuration of 0 stops ticks until sleep is cleared, and clearing it
// wakes whoever is blocked in Wait.
func setSleepPlatform(st *SleepyTicker, enabled bool) bool {
	st.sleepMode = enabled
	if enabled {
		if st.sleepDuration > 0 {
			st.ticker.Reset(st.sleepDuration)
		} else if st.sleepChannel == nil {
			// Next call to Wait blocks until SetSleep is called with enabled == false.
			st.sleepChannel = make(chan time.Time)
		}
		return true
	}

	st.ticker.Reset(st.normalDuration)
	if st.sleepDuration == 0 && st.sleepChannel != nil {
		close(st.sleepChannel)
		st.sleepChannel = nil
	}
	return true
}
