//go:build android

package atun

import (
	"errors"
	"testing"

	pmpacket "github.com/safing/portmaster/service/network/packet"
)

func TestPacketVerdictSet(t *testing.T) {
	q := &Queue{id: 17040}
	pkt := q.newPacket(pmpacket.Info{Protocol: pmpacket.TCP})

	if pkt.VerdictSet() {
		t.Fatal("verdict set on a fresh packet")
	}
	// decide records a drop when the firewall does not answer in time.
	if err := pkt.Drop(); err != nil {
		t.Fatal(err)
	}
	if !pkt.VerdictSet() || pkt.appliedMark() != MarkDrop {
		t.Fatalf("after drop: set=%v mark=%d", pkt.VerdictSet(), pkt.appliedMark())
	}
	// The firewall's late verdict must not replace the drop.
	if err := pkt.Accept(); !errors.Is(err, ErrVerdictAlreadySet) {
		t.Fatalf("late accept: got %v, want ErrVerdictAlreadySet", err)
	}
	if pkt.appliedMark() != MarkDrop {
		t.Fatalf("late accept changed the mark to %d", pkt.appliedMark())
	}
}
