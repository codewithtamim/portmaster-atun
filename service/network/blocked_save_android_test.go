//go:build android

package network

import (
	"net"
	"testing"
	"time"

	"github.com/safing/portmaster/service/intel"
	"github.com/safing/portmaster/service/network/packet"
)

func refusedConn(profile string, ip string, port uint16, verdict Verdict) *Connection {
	return &Connection{
		Type:           IPConnection,
		IPProtocol:     packet.TCP,
		Verdict:        verdict,
		Entity:         &intel.Entity{IP: net.ParseIP(ip), Port: port},
		ProcessContext: ProcessContext{Source: "local", Profile: profile},
	}
}

func TestEndRefusedOnlyEndsRefusedConnections(t *testing.T) { //nolint:paralleltest // package state.
	blocked := refusedConn("tg", "91.108.56.174", 443, VerdictBlock)
	endRefused(blocked)
	if blocked.Ended == 0 {
		t.Fatal("a blocked connection was not marked ended")
	}

	accepted := refusedConn("tg", "91.108.56.174", 443, VerdictAccept)
	endRefused(accepted)
	if accepted.Ended != 0 {
		t.Fatal("an accepted connection was marked ended")
	}

	inbound := refusedConn("tg", "91.108.56.174", 443, VerdictBlock)
	inbound.Inbound = true
	endRefused(inbound)
	if inbound.Ended != 0 {
		t.Fatal("an inbound connection was marked ended")
	}

	ended := refusedConn("tg", "91.108.56.174", 443, VerdictDrop)
	ended.Ended = 42
	endRefused(ended)
	if ended.Ended != 42 {
		t.Fatal("an existing end time was overwritten")
	}
}

func TestSkipRepeatedBlock(t *testing.T) { //nolint:paralleltest // package state.
	recentBlocksLock.Lock()
	recentBlocks = make(map[string]int64)
	recentBlocksLock.Unlock()

	if skipRepeatedBlock(refusedConn("tg", "91.108.56.174", 443, VerdictBlock)) {
		t.Fatal("the first blocked attempt was skipped")
	}
	if !skipRepeatedBlock(refusedConn("tg", "91.108.56.174", 443, VerdictBlock)) {
		t.Fatal("an identical attempt within the window was recorded")
	}
	// Other port, other destination, other app: each recorded.
	for _, c := range []*Connection{
		refusedConn("tg", "91.108.56.174", 5222, VerdictBlock),
		refusedConn("tg", "149.154.167.51", 443, VerdictBlock),
		refusedConn("other", "91.108.56.174", 443, VerdictBlock),
	} {
		if skipRepeatedBlock(c) {
			t.Fatalf("a different attempt was skipped: %s:%d", c.Entity.IP, c.Entity.Port)
		}
	}
	// Accepted connections and DNS are never skipped.
	for range 2 {
		if skipRepeatedBlock(refusedConn("tg", "91.108.56.174", 443, VerdictAccept)) {
			t.Fatal("an accepted connection was skipped")
		}
		if skipRepeatedBlock(refusedConn("tg", "10.111.222.3", 53, VerdictBlock)) {
			t.Fatal("a DNS connection was skipped")
		}
	}

	// After the window the attempt is recorded again.
	recentBlocksLock.Lock()
	for k := range recentBlocks {
		recentBlocks[k] = time.Now().Add(-2 * repeatedBlockWindow).UnixNano()
	}
	recentBlocksLock.Unlock()
	if skipRepeatedBlock(refusedConn("tg", "91.108.56.174", 443, VerdictBlock)) {
		t.Fatal("an attempt after the window was skipped")
	}
}
