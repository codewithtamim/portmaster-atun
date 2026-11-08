//go:build android

package atun

import (
	"context"
	"errors"
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
		f, err := flows.begin(string(rune('a'+i)), pmpacket.Info{})
		if err != nil {
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

	if _, err := flows.begin("dup", pmpacket.Info{}); err != nil {
		t.Fatalf("first begin was rejected: %s", err)
	}
	if _, err := flows.begin("dup", pmpacket.Info{}); !errors.Is(err, errFlowExists) {
		t.Errorf("second begin for the same id: err = %v, want errFlowExists", err)
	}
}

func TestFlowTableEndRetiresAndGetMisses(t *testing.T) {
	if err := InitFlowTable(); err != nil {
		t.Fatalf("InitFlowTable: %s", err)
	}
	t.Cleanup(TeardownFlowTable)

	f, err := flows.begin("one", pmpacket.Info{})
	if err != nil {
		t.Fatalf("begin was rejected: %s", err)
	}

	f.close()

	if _, ok := flows.get("one"); ok {
		t.Error("a closed flow is still live in the table")
	}
	if got := len(flows.all()); got != 1 {
		t.Errorf("len(all()) = %d, want 1 (the retired flow)", got)
	}
}
