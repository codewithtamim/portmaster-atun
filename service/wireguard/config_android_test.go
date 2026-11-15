//go:build android

package wireguard

import (
	"testing"

	"github.com/safing/portmaster/service/routing"
)

func TestWantsTunnel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		profile  string
		selected bool
		want     bool
	}{
		// No config at all: the target has nothing to do, traffic goes direct.
		{"nothing", "", false, false},
		// After an engine restart, before the app installed the profile again.
		{"selected, not installed", "", true, true},
		{"installed", "Vienna", true, true},
		// Installed before the selection was ever saved.
		{"installed, not saved", "Vienna", false, true},
	}
	for _, tc := range tests {
		if got := wantsTunnel(tc.profile, tc.selected); got != tc.want {
			t.Errorf("%s: wantsTunnel = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestConfigSelectedBeforeRegister(t *testing.T) {
	t.Parallel()

	// The option is registered by New, which tests do not run. Reading and
	// writing it must then be harmless, not a panic.
	if configSelected != nil {
		t.Skip("config registered")
	}
	if ConfigSelected() {
		t.Error("ConfigSelected() = true with no option registered")
	}
	setConfigSelected(true)
}

// Removing the selected config must not open a direct path while WireGuard is
// the routing target: the tunnel stays wanted, so the firewall keeps failing
// closed. On any other target the selection just ends.
func TestKeepsSelectionAfterClear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   routing.Target
		selected bool
		want     bool
	}{
		{"wireguard target, config was selected", routing.TargetWireGuard, true, true},
		{"wireguard target, never had a config", routing.TargetWireGuard, false, false},
		{"direct target, config was selected", routing.TargetDirect, true, false},
		{"spn target, config was selected", routing.TargetSPN, true, false},
	}
	for _, tc := range tests {
		if got := keepsSelectionAfterClear(tc.target, tc.selected); got != tc.want {
			t.Errorf("%s: keepsSelectionAfterClear = %t, want %t", tc.name, got, tc.want)
		}
		if got := configRemoved(tc.target, tc.selected); got != tc.want {
			t.Errorf("%s: configRemoved = %t, want %t", tc.name, got, tc.want)
		}
	}
}
