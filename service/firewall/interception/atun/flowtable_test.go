//go:build android

package atun

import (
	"context"
	"testing"

	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// The retired list is drained by the bandwidth reporter. A congested update
// queue must not stop that, or the list grows for as long as the congestion
// lasts and only a VPN restart recovers.
func TestReportBandwidthReapsRetiredWhenQueueIsFull(t *testing.T) {
	if err := InitFlowTable(); err != nil {
		t.Fatalf("InitFlowTable: %s", err)
	}
	t.Cleanup(TeardownFlowTable)

	// One slot, filled, so every send hits the default branch.
	updates := make(chan *pmpacket.BandwidthUpdate, 1)
	updates <- &pmpacket.BandwidthUpdate{ConnID: "filler"}

	const count = 20
	for i := range count {
		f, ok := flows.begin(string(rune('a'+i)), pmpacket.Info{})
		if !ok {
			t.Fatalf("flow %d was not tracked", i)
		}
		// Unreported bytes, so each flow wants to send an update.
		f.rx.Store(1)
		f.close()
	}

	if got := len(flows.retired); got != count {
		t.Fatalf("len(retired) = %d before reporting, want %d", got, count)
	}

	if !reportBandwidth(context.Background(), updates) {
		t.Fatal("reportBandwidth reported the worker should stop")
	}

	if got := len(flows.retired); got != 0 {
		t.Errorf("len(retired) = %d after reporting, want 0", got)
	}
}

func TestFlowTableBeginIsIdempotentPerID(t *testing.T) {
	if err := InitFlowTable(); err != nil {
		t.Fatalf("InitFlowTable: %s", err)
	}
	t.Cleanup(TeardownFlowTable)

	if _, ok := flows.begin("dup", pmpacket.Info{}); !ok {
		t.Fatal("first begin was rejected")
	}
	if _, ok := flows.begin("dup", pmpacket.Info{}); ok {
		t.Error("second begin for the same id was accepted")
	}
}

func TestFlowTableEndRetiresAndGetMisses(t *testing.T) {
	if err := InitFlowTable(); err != nil {
		t.Fatalf("InitFlowTable: %s", err)
	}
	t.Cleanup(TeardownFlowTable)

	f, ok := flows.begin("one", pmpacket.Info{})
	if !ok {
		t.Fatal("begin was rejected")
	}

	f.close()

	if _, ok := flows.get("one"); ok {
		t.Error("a closed flow is still live in the table")
	}
	if got := len(flows.all()); got != 1 {
		t.Errorf("len(all()) = %d, want 1 (the retired flow)", got)
	}
}
