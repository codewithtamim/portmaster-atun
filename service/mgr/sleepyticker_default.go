//go:build !android

package mgr

// setSleepPlatform leaves SetSleep to the upstream logic.
// TODO: upstream SetSleep(false) on a ticker with a sleepDuration above 0
// closes sleepChannel, which is nil there, and panics. Nothing on desktop
// sets sleep mode today.
func setSleepPlatform(*SleepyTicker, bool) bool { return false }
