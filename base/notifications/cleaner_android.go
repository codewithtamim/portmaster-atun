//go:build android

package notifications

import "time"

// cleanerInterval is how often expired notifications are removed. Expiry is
// in whole seconds and nothing on Android needs it removed the same second;
// a 1 s ticker would wake the CPU 3600 times an hour for that.
const cleanerInterval = 30 * time.Second
