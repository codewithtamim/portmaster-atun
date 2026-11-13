//go:build android

package netquery

// maxRetentionDays is the longest the phone keeps connection history.
const maxRetentionDays int64 = 30

// cappedRetentionDays caps how many days of history a profile keeps. On a phone the
// history must not grow without end, so "keep forever" (0) and anything longer
// than the cap both keep the cap instead.
func cappedRetentionDays(days int64) int64 {
	if days <= 0 || days > maxRetentionDays {
		return maxRetentionDays
	}
	return days
}
