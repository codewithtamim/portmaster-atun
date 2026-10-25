//go:build android

package atun

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/network"
	pmpacket "github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/spn/crew"
)

// verdictTimeout bounds the wait for a firewall verdict, so a stuck firewall
// cannot pin gVisor endpoints. A flow that times out is dropped.
const verdictTimeout = 10 * time.Second

type flow struct {
	id   string
	info pmpacket.Info

	mark atomic.Int32

	rx         atomic.Uint64
	tx         atomic.Uint64
	reportedRx atomic.Uint64
	reportedTx atomic.Uint64

	mu      sync.Mutex
	closers []io.Closer
	closed  bool
}

func (f *flow) appliedMark() int {
	return int(f.mark.Load())
}

// addCloser registers c to be closed with the flow. If the flow has already
// closed, c is closed now, so a dial that finishes after teardown cannot leak.
func (f *flow) addCloser(c io.Closer) {
	if c == nil {
		return
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		_ = c.Close()

		return
	}
	f.closers = append(f.closers, c)
	f.mu.Unlock()
}

// close is idempotent. It moves the flow to the retired list first, so the
// bandwidth reporter still sends its final counters, then closes the sockets.
func (f *flow) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()

		return
	}
	f.closed = true
	closers := f.closers
	f.closers = nil
	f.mu.Unlock()

	flows.end(f.id)

	for _, c := range closers {
		_ = c.Close()
	}
}

func (f *flow) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closed
}

// countingConn counts bytes on the app side: tx is what the app sent, rx what
// it received.
type countingConn struct {
	net.Conn

	flow *flow
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.flow.tx.Add(uint64(n))
	}

	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.flow.rx.Add(uint64(n))
	}

	return n, err
}

func (c *countingConn) CloseRead() error {
	if cr, ok := c.Conn.(closeReader); ok {
		return cr.CloseRead()
	}
	return nil
}

func (c *countingConn) CloseWrite() error {
	if cw, ok := c.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return nil
}

// decide submits the flow and waits for its verdict. It fails closed: no
// verdict, for any reason, is MarkDrop. The caller enforces the mark.
func decide(s *stackSession, q *Queue, f *flow) int {
	pkt := q.newPacket(f.info)

	if !q.submit(pkt) {
		return MarkDrop
	}

	select {
	case <-pkt.verdictSet:
	case <-time.After(verdictTimeout):
		log.Warningf("atun: verdict for %s timed out after %s, dropping", pkt, verdictTimeout)
		// Record the drop on the packet so the firewall skips it if it is
		// still queued. A verdict landing at this instant changes nothing:
		// the flow has timed out and is dropped either way.
		_ = pkt.Drop()

		return MarkDrop
	case <-s.ctx.Done():
		_ = pkt.Drop()
		return MarkDrop
	}

	mark := pkt.appliedMark()
	f.mark.Store(int32(mark))
	log.Tracef("atun: %s verdict %s", pkt, markToString(mark))

	return mark
}

func markIsAccept(mark int) bool {
	switch mark {
	case MarkAccept, MarkAcceptAlways, MarkAcceptFinal:
		return true
	}

	return false
}

func markIsBlock(mark int) bool {
	switch mark {
	case MarkBlock, MarkBlockAlways:
		return true
	}

	return false
}

// ownedConn ties the flow's lifetime to the SPN's use of the connection:
// whoever closes it also untracks the flow.
type ownedConn struct {
	net.Conn

	flow *flow
}

func (c *ownedConn) Close() error {
	c.flow.close()

	return c.Conn.Close()
}

// handOffToSPN gives the connection to crew directly. Sluice is skipped
// because it accepts redirected kernel sockets, and here there is none.
// Crew owns the connection afterwards; the flow stays tracked for bandwidth.
func handOffToSPN(f *flow, appConn net.Conn) bool {
	conn, ok := network.GetConnection(f.id)
	if !ok {
		log.Warningf("atun: no connection found for tunnelled flow %s", f.id)

		return false
	}

	counted := &countingConn{Conn: appConn, flow: f}
	owned := &ownedConn{Conn: counted, flow: f}
	f.addCloser(owned)
	crew.HandleSluiceRequest(conn, owned)

	return true
}
