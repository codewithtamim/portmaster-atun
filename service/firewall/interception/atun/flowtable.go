//go:build android

package atun

import (
	"sync"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/network"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// In-process flow table. There is no kernel conntrack on this platform:
// closing a flow tears down its proxy connections, so the app dials again and
// gets a fresh verdict.

// flows is the session table. Emptied by TeardownFlowTable each session.
var flows = &flowTable{}

type flowTable struct {
	mu      sync.RWMutex
	flows   map[string]*flow
	retired []*flow
}

// begin tracks a new flow. False means the tuple already has one: a SYN
// retransmit or a duplicate UDP endpoint.
func (t *flowTable) begin(connID string, info pmpacket.Info) (*flow, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.flows == nil {
		t.flows = make(map[string]*flow)
	}
	if _, exists := t.flows[connID]; exists {
		return nil, false
	}

	f := &flow{id: connID, info: info}
	t.flows[connID] = f

	return f, true
}

func (t *flowTable) end(connID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if f, ok := t.flows[connID]; ok {
		delete(t.flows, connID)
		t.retired = append(t.retired, f)
	}
}

// maxUDPFlows caps live UDP flows. Each holds a gVisor endpoint, an upstream
// socket and two workers until its idle timeout, and UDP never signals an end.
const maxUDPFlows = 256

func (t *flowTable) udpCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := 0
	for _, f := range t.flows {
		if f.info.Protocol == pmpacket.UDP {
			n++
		}
	}
	return n
}

func (t *flowTable) get(connID string) (*flow, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	f, ok := t.flows[connID]

	return f, ok
}

func (t *flowTable) all() []*flow {
	t.mu.RLock()
	defer t.mu.RUnlock()

	all := make([]*flow, 0, len(t.flows)+len(t.retired))
	for _, f := range t.flows {
		all = append(all, f)
	}
	all = append(all, t.retired...)

	return all
}

func init() {
	network.SetFlowAlive(flowIsLive)
}

// flowIsLive reports whether the TUN stack is still proxying this connection.
func flowIsLive(id string) bool {
	f, ok := flows.get(id)
	if !ok {
		return false
	}
	return !f.isClosed()
}

func InitFlowTable() error {
	flows.mu.Lock()
	defer flows.mu.Unlock()

	flows.flows = make(map[string]*flow)
	flows.retired = nil

	return nil
}

func TeardownFlowTable() {
	flows.mu.Lock()
	tracked := make([]*flow, 0, len(flows.flows)+len(flows.retired))
	for _, f := range flows.flows {
		tracked = append(tracked, f)
	}
	tracked = append(tracked, flows.retired...)
	flows.flows = nil
	flows.retired = nil
	flows.mu.Unlock()

	for _, f := range tracked {
		f.close()
	}
}

func (t *flowTable) removeRetired(target *flow) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, f := range t.retired {
		if f == target {
			t.retired = append(t.retired[:i], t.retired[i+1:]...)
			return
		}
	}
}

// DeleteUnmarkedConnections closes live flows that never received a verdict.
func DeleteUnmarkedConnections() error {
	var deleted int
	for _, f := range flows.all() {
		if f.appliedMark() == 0 {
			f.close()
			deleted++
		}
	}

	if deleted > 0 {
		log.Debugf("atun: deleted %d unmarked connections", deleted)
	}

	return nil
}

// DeleteAllMarkedConnection closes live flows that already have a verdict.
func DeleteAllMarkedConnection() error {
	var deleted int
	for _, f := range flows.all() {
		if f.appliedMark() != 0 {
			f.close()
			deleted++
		}
	}

	if deleted > 0 {
		log.Debugf("atun: deleted %d marked connections", deleted)
	}

	return nil
}

// DeleteMarkedConnection closes the flow for the given connection.
func DeleteMarkedConnection(conn *network.Connection) error {
	if conn == nil {
		return nil
	}

	f, ok := flows.get(conn.ID)
	if !ok {
		return nil
	}
	f.close()

	return nil
}
