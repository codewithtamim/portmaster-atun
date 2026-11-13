//go:build !android

package netquery

// cappedRetentionDays returns the configured days unchanged on desktop, where 0
// keeps history forever.
func cappedRetentionDays(days int64) int64 {
	return days
}
