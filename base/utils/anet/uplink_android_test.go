//go:build android

package anet

import (
	"testing"
	"time"
)

func ifaces(uplink string, names ...string) []NetworkInterface {
	list := make([]NetworkInterface, 0, len(names))
	for _, name := range names {
		list = append(list, NetworkInterface{Name: name, Up: true, Uplink: name == uplink})
	}
	return list
}

func signalled(t *testing.T, ch <-chan struct{}) bool {
	t.Helper()
	select {
	case <-ch:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestUplinkMoveIsSignalled(t *testing.T) { //nolint:paralleltest // package state.
	t.Cleanup(func() { UpdateInterfaces(nil); mu.Lock(); uplinkName = ""; mu.Unlock() })
	mu.Lock()
	uplinkName = ""
	mu.Unlock()
	// A new flag starts set; Refresh takes the current state, as the SPN
	// client does before it waits on it.
	flag := UplinkChangedFlag()
	flag.Refresh()

	// First learning the uplink is not a move.
	UpdateInterfaces(ifaces("rmnet1", "rmnet1", "lo"))
	if signalled(t, flag.Signal()) {
		t.Fatal("first uplink signalled a move")
	}
	if got := UplinkName(); got != "rmnet1" {
		t.Fatalf("uplink = %q, want rmnet1", got)
	}

	// Wi-Fi appears but Android has not moved the uplink yet.
	UpdateInterfaces(ifaces("rmnet1", "rmnet1", "wlan0", "lo"))
	if signalled(t, flag.Signal()) {
		t.Fatal("a new interface alone signalled a move")
	}

	// Android moves the uplink to Wi-Fi.
	UpdateInterfaces(ifaces("wlan0", "rmnet1", "wlan0", "lo"))
	if !signalled(t, flag.Signal()) {
		t.Fatal("moving the uplink to wlan0 was not signalled")
	}
	flag.Refresh()

	// Losing the uplink for a moment is not a move, and coming back to the
	// same network neither.
	UpdateInterfaces(ifaces("", "rmnet1", "wlan0", "lo"))
	UpdateInterfaces(ifaces("wlan0", "rmnet1", "wlan0", "lo"))
	if signalled(t, flag.Signal()) {
		t.Fatal("a gap on the same uplink signalled a move")
	}

	// Back to mobile data.
	UpdateInterfaces(ifaces("rmnet1", "rmnet1", "lo"))
	if !signalled(t, flag.Signal()) {
		t.Fatal("moving the uplink back to rmnet1 was not signalled")
	}
}
