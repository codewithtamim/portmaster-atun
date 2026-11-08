//go:build android

package wireguard

import "testing"

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
