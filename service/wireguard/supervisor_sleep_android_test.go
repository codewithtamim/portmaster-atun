//go:build android

package wireguard

import "testing"

func TestConnectedTunnelIsCheckedLessOftenWhileAsleep(t *testing.T) {
	t.Parallel()

	w := &WireGuard{wake: make(chan struct{}, 1)}
	if got := w.connectedCheckInterval(); got != connectedInterval {
		t.Fatalf("awake interval = %s, want %s", got, connectedInterval)
	}

	w.SetSleep(true)
	if got := w.connectedCheckInterval(); got != idleInterval {
		t.Fatalf("asleep interval = %s, want %s", got, idleInterval)
	}
	select {
	case <-w.wake:
		t.Fatal("falling asleep must not wake the supervisor")
	default:
	}

	w.SetSleep(false)
	if got := w.connectedCheckInterval(); got != connectedInterval {
		t.Fatalf("interval after waking = %s, want %s", got, connectedInterval)
	}
	select {
	case <-w.wake:
	default:
		t.Fatal("waking up must check the tunnel at once")
	}
}
