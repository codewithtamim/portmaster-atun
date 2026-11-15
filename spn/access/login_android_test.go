//go:build android

package access

import (
	"errors"
	"testing"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/spn/access/account"
)

// Signing in must leave spn/enable alone on Android. The routing target owns
// that option there, and a sign-in that flipped it would move a WireGuard or
// direct user onto the SPN without being asked.
func TestSignInDoesNotEnableSPN(t *testing.T) {
	t.Parallel()

	if EnableAfterLogin {
		t.Fatal("sign-in would turn the SPN on by itself")
	}
}

// A refused profile refresh must carry the account server's status for the
// two device states the app explains to the user, and nothing else.
func TestProfileRefreshErrorKeepsDeviceStatus(t *testing.T) {
	t.Parallel()

	for _, code := range []int{account.StatusReachedDeviceLimit, account.StatusDeviceInactive} {
		var status *api.HTTPStatusError
		if err := profileRefreshError(ErrDeviceLimitReached, code); !errors.As(err, &status) || status.HTTPStatus() != code {
			t.Errorf("status %d was not kept: %v", code, err)
		}
	}

	var status *api.HTTPStatusError
	if err := profileRefreshError(ErrInvalidCredentials, account.StatusInvalidAuth); errors.As(err, &status) {
		t.Errorf("status %d must stay a plain error", account.StatusInvalidAuth)
	}
}
