//go:build android

package atun

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

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

	// Live flow counts for the caps, guarded by mu.
	total map[pmpacket.IPProtocol]int
	byUID map[uidKey]int
}

type uidKey struct {
	protocol pmpacket.IPProtocol
	uid      int
}

var (
	errFlowExists = errors.New("flow already tracked")
	errFlowLimit  = errors.New("flow limit reached")
)

// Flow caps. A UDP flow holds a gVisor endpoint, an upstream socket and two
// workers until its idle timeout, as UDP never signals an end. A TCP flow
// holds up to two 1 MiB gVisor buffers, two relay buffers, two workers and an
// fd. The per-app cap stops one app from starving the others; the total is
// only a backstop. DNS is exempt, see flowIsCapped.
// Vars, not consts, so tests can lower them.
var (
	maxUDPFlowsPerUID = 128
	maxUDPFlows       = 1024
	maxTCPFlowsPerUID = 256
	maxTCPFlows       = 1024
)

func flowLimits(protocol pmpacket.IPProtocol) (total, perUID int) {
	switch protocol { //nolint:exhaustive
	case pmpacket.UDP:
		return maxUDPFlows, maxUDPFlowsPerUID
	case pmpacket.TCP:
		return maxTCPFlows, maxTCPFlowsPerUID
	}

	return 0, 0
}

// flowIsCapped reports whether info counts against the caps. DNS does not:
// system DNS to the fake DNS address and DNS the firewall reroutes to the
// nameserver must keep working however many flows an app holds. DNS flows
// are short lived, see dnsIdleTimeout.
func flowIsCapped(info pmpacket.Info) bool {
	total, _ := flowLimits(info.Protocol)
	return total > 0 && !isDNSPort(info)
}

// begin tracks a new flow. errFlowExists means the tuple already has one: a
// SYN retransmit or a duplicate UDP endpoint. errFlowLimit means the UDP total
// is reached. Count and insert happen under one lock.
//
// Only UDP is counted here, as its endpoint exists before the verdict. A TCP
// flow holds nothing until it is accepted, and gVisor already bounds pending
// handshakes, so it is counted in charge. Else one app's SYN burst waiting
// for verdicts could fill the total for everyone.
func (t *flowTable) begin(connID string, info pmpacket.Info) (*flow, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.flows == nil {
		t.flows = make(map[string]*flow)
	}
	if _, exists := t.flows[connID]; exists {
		return nil, errFlowExists
	}

	f := &flow{id: connID, info: info, ownerUID: -1}
	if info.Protocol == pmpacket.UDP && flowIsCapped(info) {
		if !t.chargeTotalLocked(f) {
			return nil, errFlowLimit
		}
	}
	t.flows[connID] = f

	return f, nil
}

// charge counts f against the caps once its owner is known. It also counts
// a port 53 flow that turned out not to be DNS for us. A nil or negative uid
// is only checked against the total, so unattributed flows of different apps
// do not share one per-app budget. False means a cap is reached and the
// caller must refuse the flow.
func (t *flowTable) charge(f *flow, uid *int) bool {
	total, perUID := flowLimits(f.info.Protocol)
	if total == 0 {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.flows[f.id] != f {
		// Closed meanwhile; end has run and must not be undone.
		return false
	}

	chargedTotal := false
	if !f.countedTotal {
		if !t.chargeTotalLocked(f) {
			return false
		}
		chargedTotal = true
	}

	if uid == nil || *uid < 0 || f.countedUID {
		return true
	}

	key := uidKey{protocol: f.info.Protocol, uid: *uid}
	if t.byUID[key] >= perUID {
		if chargedTotal {
			t.releaseTotalLocked(f)
		}
		return false
	}
	if t.byUID == nil {
		t.byUID = make(map[uidKey]int)
	}
	t.byUID[key]++
	f.countedUID = true
	f.ownerUID = *uid

	return true
}

func (t *flowTable) chargeTotalLocked(f *flow) bool {
	total, _ := flowLimits(f.info.Protocol)
	if t.total[f.info.Protocol] >= total {
		return false
	}
	if t.total == nil {
		t.total = make(map[pmpacket.IPProtocol]int)
	}
	t.total[f.info.Protocol]++
	f.countedTotal = true

	return true
}

func (t *flowTable) releaseTotalLocked(f *flow) {
	if !f.countedTotal {
		return
	}
	f.countedTotal = false
	if t.total[f.info.Protocol]--; t.total[f.info.Protocol] <= 0 {
		delete(t.total, f.info.Protocol)
	}
}

func (t *flowTable) releaseLocked(f *flow) {
	t.releaseTotalLocked(f)
	if f.countedUID {
		f.countedUID = false
		key := uidKey{protocol: f.info.Protocol, uid: f.ownerUID}
		if t.byUID[key]--; t.byUID[key] <= 0 {
			delete(t.byUID, key)
		}
	}
}

// counts returns the live counted flows of protocol, in total and for uid.
func (t *flowTable) counts(protocol pmpacket.IPProtocol, uid int) (total, perUID int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.total[protocol], t.byUID[uidKey{protocol: protocol, uid: uid}]
}

func (t *flowTable) end(connID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if f, ok := t.flows[connID]; ok {
		delete(t.flows, connID)
		t.releaseLocked(f)
		t.retired = append(t.retired, f)
	}
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
	flows.total = nil
	flows.byUID = nil

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
	flows.total = nil
	flows.byUID = nil
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

// limitLog logs at most once per interval, so a flood of refused flows does
// not flood the log too.
type limitLog struct {
	mu      sync.Mutex
	last    time.Time
	skipped int
}

const limitLogInterval = 10 * time.Second

func (l *limitLog) warn(format string, args ...any) {
	l.mu.Lock()
	if !l.last.IsZero() && time.Since(l.last) < limitLogInterval {
		l.skipped++
		l.mu.Unlock()

		return
	}
	skipped := l.skipped
	l.skipped = 0
	l.last = time.Now()
	l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	if skipped > 0 {
		msg = fmt.Sprintf("%s (%d more not logged)", msg, skipped)
	}
	log.Warning(msg)
}

var flowLimitLog limitLog

func uidString(uid *int) string {
	if uid == nil {
		return "unknown"
	}
	return strconv.Itoa(*uid)
}
