package mgr

import (
	"testing"
	"time"
)

// TestSleepyTickerWakeWithSleepDuration covers clearing sleep on a ticker that
// ticks during sleep. Such a ticker never creates a sleep channel, so waking it
// must not try to close one.
func TestSleepyTickerWakeWithSleepDuration(t *testing.T) {
	st := NewSleepyTicker(100*time.Millisecond, 200*time.Millisecond)
	defer st.Stop()

	st.SetSleep(true)
	st.SetSleep(false) // must not panic

	select {
	case <-st.Wait():
		// Expected tick at the normal duration.
	case <-time.After(500 * time.Millisecond):
		t.Error("expected tick after waking from sleep")
	}
}

// TestSleepyTickerRepeatedSleep covers repeated sleep toggles on a ticker that
// stops during sleep. The sleep channel must be recreated after each wake.
func TestSleepyTickerRepeatedSleep(t *testing.T) {
	st := NewSleepyTicker(100*time.Millisecond, 0)
	defer st.Stop()

	for range 3 {
		st.SetSleep(true)
		st.SetSleep(true) // must not replace the channel
		st.SetSleep(false)
		st.SetSleep(false) // must not close twice
	}

	st.SetSleep(true)
	select {
	case <-st.Wait():
		t.Error("did not expect tick when sleepDuration is 0")
	case <-time.After(200 * time.Millisecond):
		// Expected no tick.
	}
}
