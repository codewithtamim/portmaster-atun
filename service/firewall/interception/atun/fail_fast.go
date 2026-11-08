//go:build android

package atun

import (
	"sync"
	"time"
)

// A blocked flow is normally refused at once (a TCP reset, an ICMP error for
// UDP), so the app fails fast and does not wait for a timeout. An app that
// retries in a tight loop turns that into a flood: one blocked messenger was
// seen at over a thousand attempts a second, each going through the whole
// firewall, which kept the core busy enough for its API to time out.
//
// Each app gets a small budget of fast refusals. Past it, blocked flows are
// dropped silently: the app's connect then waits for its own timeout, which
// slows the loop to a few attempts per destination, and the flows stay
// blocked either way.
const (
	failFastBurst  = 20 // refusals an app may get at once
	failFastPerSec = 5  // refusals an app gets back per second
	// failFastIdle drops buckets of apps that stopped retrying.
	failFastIdle = time.Minute
)

type failFastBucket struct {
	tokens float64
	last   time.Time
}

var (
	failFastLock    sync.Mutex
	failFastBuckets = make(map[int]*failFastBucket)
	failFastSwept   time.Time
)

// failFast reports whether a blocked flow of the app with this uid should be
// refused at once (true) or dropped silently (false). Flows without a known
// owner are always refused at once.
func failFast(uid *int) bool {
	if uid == nil {
		return true
	}
	return failFastAt(*uid, time.Now())
}

func failFastAt(uid int, now time.Time) bool {
	failFastLock.Lock()
	defer failFastLock.Unlock()

	if now.Sub(failFastSwept) > failFastIdle {
		for id, b := range failFastBuckets {
			if now.Sub(b.last) > failFastIdle {
				delete(failFastBuckets, id)
			}
		}
		failFastSwept = now
	}

	b, ok := failFastBuckets[uid]
	if !ok {
		b = &failFastBucket{tokens: failFastBurst, last: now}
		failFastBuckets[uid] = b
	}
	b.tokens = min(failFastBurst, b.tokens+now.Sub(b.last).Seconds()*failFastPerSec)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
