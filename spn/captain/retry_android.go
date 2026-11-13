//go:build android

package captain

import (
	"time"

	"github.com/safing/portmaster/spn/access"
)

// signedOutRetryLimit caps the wait while nobody is signed in, so a missed
// account event does not leave the client parked for good.
const signedOutRetryLimit = 10 * time.Minute

// clientRetryWait returns how long to wait before connecting again, and a
// channel that ends the wait early. On Android the SPN stays the routing
// target after sign-out, so without an account there is nothing to retry
// until someone signs in: the client waits for the account to change
// instead of waking the phone every few seconds.
func clientRetryWait() (time.Duration, <-chan struct{}) {
	user, err := access.GetUser()
	if err == nil && user.IsLoggedIn() {
		return clientRetryConnectBackoffDuration, nil
	}
	return signedOutRetryLimit, access.AccountChanged()
}
