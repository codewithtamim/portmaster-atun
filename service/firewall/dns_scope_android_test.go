//go:build android

package firewall

import (
	"testing"

	"github.com/safing/portmaster/service/profile"
)

func TestSystemLookupsAreSharedGlobally(t *testing.T) {
	if !sharesIPInfoGlobally(profile.SystemProfileID) {
		t.Fatal("lookups under the system identity must be stored for every app")
	}
	if sharesIPInfoGlobally("local/com.example.app") {
		t.Fatal("an app's own lookups must stay in its own scope")
	}
}
