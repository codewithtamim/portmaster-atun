//go:build !android

package notifications

import "time"

// cleanerInterval is how often expired notifications are removed.
const cleanerInterval = 1 * time.Second
