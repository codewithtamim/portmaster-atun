//go:build android

package firewall

import (
	"testing"

	"github.com/safing/portmaster/service/network"
)

func TestPreAuthOwnerMatchesOnlyOwnUID(t *testing.T) {
	own := ownUID
	other := ownUID + 1

	if !preAuthOwnerMatches(&network.Connection{UID: &own}) {
		t.Error("own UID was refused")
	}
	if preAuthOwnerMatches(&network.Connection{UID: &other}) {
		t.Error("another app's UID was accepted")
	}
	if preAuthOwnerMatches(&network.Connection{}) {
		t.Error("a connection with no UID was accepted")
	}
}

// A foreign app on the same port must not use up our entry.
func TestPreAuthForeignAppKeepsEntry(t *testing.T) {
	const proto, port = 17, 53001
	registerPreAuthenticatedPort(proto, port)
	t.Cleanup(func() { unregisterPreAuthenticatedPort(proto, port) })

	other := ownUID + 1
	foreign := &network.Connection{UID: &other}
	if preAuthOwnerMatches(foreign) && localPortIsPreAuthenticated(proto, port) {
		t.Fatal("foreign app was granted the pre-authenticated port")
	}

	own := ownUID
	mine := &network.Connection{UID: &own}
	if !(preAuthOwnerMatches(mine) && localPortIsPreAuthenticated(proto, port)) {
		t.Fatal("own connection lost its pre-authenticated port")
	}
}
