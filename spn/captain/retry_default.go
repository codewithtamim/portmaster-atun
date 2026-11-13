//go:build !android

package captain

import "time"

// clientRetryWait keeps the fixed retry wait on desktop. The nil channel
// never fires.
func clientRetryWait() (time.Duration, <-chan struct{}) {
	return clientRetryConnectBackoffDuration, nil
}
