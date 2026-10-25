//go:build android

// Package atun intercepts connections from the Android VPN TUN.
package atun

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
	"github.com/safing/portmaster/service/mgr"
	pmpacket "github.com/safing/portmaster/service/network/packet"
)

// Errors.
var (
	ErrInvalidFD         = errors.New("atun: invalid TUN fd")
	ErrInvalidMTU        = errors.New("atun: invalid TUN mtu")
	ErrStackClosed       = errors.New("atun: stack closed")
	ErrNoTunConfig       = errors.New("atun: no TUN configured, StartTun has not run")
	ErrInvalidDNSAddr    = errors.New("atun: invalid fake DNS address")
	ErrInvalidTunAddr    = errors.New("atun: invalid TUN address")
	ErrVerdictAlreadySet = errors.New("atun: verdict already set")
	ErrNoProtector       = errors.New("atun: socket protector not registered")
)

// packetQueueSize is nfq's queue depth, so a verdict backlog builds the same
// way it does on Linux.
const packetQueueSize = 1000

// submitTimeout is how long a forwarder waits for queue space before it drops
// the flow without a verdict, as nfqueue does when the firewall falls behind.
const submitTimeout = time.Second

var nextPacketID atomic.Uint32

var (
	sessionMu sync.Mutex
	session   *stackSession

	// sessionOpen mirrors session != nil for Running, which the app polls from
	// its main thread and must not wait on sessionMu while a stack builds.
	sessionOpen atomic.Bool
)

// Running reports whether a TUN session is open.
func Running() bool {
	return sessionOpen.Load()
}

// Open builds the gVisor stack on the TUN stored by SetTunConfig. It is a
// no-op while a session is open. Call New once per IP family afterwards.
// Fails without a socket protector: every upstream dial would loop back into
// the TUN.
func Open(workerManager *mgr.Manager) error {
	if !aprotect.HasProtector() {
		return ErrNoProtector
	}

	sessionMu.Lock()
	defer sessionMu.Unlock()

	if session != nil && !session.closed.Load() {
		return nil
	}

	cfg, ok := CurrentTunConfig()
	if !ok {
		return ErrNoTunConfig
	}
	st, err := newStack(cfg, workerManager)
	if err != nil {
		return err
	}
	session = st
	sessionOpen.Store(true)

	return nil
}

// New opens the queue for one IP family on the open session, the way
// nfq.New opens one nfqueue.
func New(qid uint16, v6 bool) (*Queue, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if session == nil {
		return nil, ErrStackClosed
	}

	return session.newQueue(qid, v6)
}

// Close tears down the session. Destroy the queues first so no new flow is
// handed to the firewall while the stack shuts down.
func Close() error {
	sessionMu.Lock()
	st := session
	session = nil
	sessionOpen.Store(false)
	sessionMu.Unlock()

	if st == nil {
		return nil
	}

	return st.Close()
}

// Queue hands one IP family's flows to the firewall. Unlike nfqueue there is
// no kernel queue behind it: the gVisor forwarders submit to it directly.
type Queue struct {
	id      uint16
	v6      bool
	stack   *stackSession
	packets chan pmpacket.Packet
	closed  atomic.Bool
}

func (s *stackSession) newQueue(qid uint16, v6 bool) (*Queue, error) {
	if s == nil {
		return nil, ErrStackClosed
	}

	q := &Queue{
		id:      qid,
		v6:      v6,
		stack:   s,
		packets: make(chan pmpacket.Packet, packetQueueSize),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return nil, ErrStackClosed
	}
	if v6 {
		s.queue6 = q
	} else {
		s.queue4 = q
	}

	return q, nil
}

// Destroy stops new flows from being queued. The channel stays open, see
// stackSession.Close for why.
func (q *Queue) Destroy() {
	if q == nil {
		return
	}
	q.closed.Store(true)
}

// PacketChannel returns the flows waiting for a verdict.
func (q *Queue) PacketChannel() <-chan pmpacket.Packet {
	if q == nil {
		return nil
	}
	return q.packets
}

// submit queues pkt for a verdict. False means the caller must drop the flow:
// the queue is closed, the session ended, or the firewall did not keep up.
func (q *Queue) submit(pkt *packet) bool {
	if q == nil || q.closed.Load() {
		return false
	}

	select {
	case q.packets <- pkt:
		return true
	case <-q.stack.ctx.Done():
		return false
	case <-time.After(submitTimeout):
		log.Warningf("atun: failed to queue packet %s after %s, dropping", pkt, submitTimeout)
		return false
	}
}

func (q *Queue) newPacket(info pmpacket.Info) *packet {
	pkt := &packet{
		pktID:          nextPacketID.Add(1),
		queue:          q,
		verdictSet:     make(chan struct{}),
		verdictPending: abool.New(),
	}
	pkt.SetPacketInfo(info)

	return pkt
}
