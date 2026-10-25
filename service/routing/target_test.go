package routing

import "testing"

func TestTargetValid(t *testing.T) {
	t.Parallel()

	valid := []Target{TargetDirect, TargetSPN, TargetWireGuard}
	for _, target := range valid {
		if !target.Valid() {
			t.Errorf("Target(%q).Valid() = false, want true", target)
		}
		if target.String() != string(target) {
			t.Errorf("Target(%q).String() = %q", target, target.String())
		}
	}

	invalid := []Target{"", "Direct", "spn ", "tunnel", "wireguard\n"}
	for _, target := range invalid {
		if target.Valid() {
			t.Errorf("Target(%q).Valid() = true, want false", target)
		}
	}
}
