//go:build android

package resolver

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/log"
)

// lastTCPRead holds, per DNS-over-TCP/TLS connection, when a reply was last
// read from it.
var lastTCPRead sync.Map // *tcpResolverConn -> *atomic.Int64 (unix nanoseconds)

// noteTCPRead records that a reply was read from the connection.
func noteTCPRead(trc *tcpResolverConn) {
	now := time.Now().UnixNano()
	if v, ok := lastTCPRead.Load(trc); ok {
		v.(*atomic.Int64).Store(now) //nolint:forcetypeassert // Only this file stores values.
		return
	}
	v := new(atomic.Int64)
	v.Store(now)
	lastTCPRead.Store(trc, v)
}

// forgetTCPConn drops the read time of a connection that has shut down.
func forgetTCPConn(trc *tcpResolverConn) {
	lastTCPRead.Delete(trc)
}

// tcpReplyTimedOut drops a DNS-over-TCP/TLS connection that went silent.
//
// A connection can stop answering without an error, for example through the
// WireGuard netstack after the path changes: writes still succeed, but the
// reader waits forever. Its unanswered queries then stay in flight, so the
// connection is never recycled and every lookup that needs the upstream
// times out until the app restarts.
//
// It is dropped only when nothing at all was read from it while the query
// waited. One slow answer on a connection that keeps answering other queries
// leaves it alone: closing it would fail every query in flight, and enough
// failures mark the only resolver as failing. Closing wakes the reader, and
// the next query dials a new connection.
func (tr *TCPResolver) tcpReplyTimedOut(trc *tcpResolverConn) {
	waitStarted := time.Now().Add(-defaultRequestTimeout).UnixNano()
	if v, ok := lastTCPRead.Load(trc); ok && v.(*atomic.Int64).Load() > waitStarted { //nolint:forcetypeassert // Only this file stores values.
		return
	}
	if !trc.abandoned.SetToIf(false, true) {
		return
	}
	log.Warningf(
		"resolver: no reply from %s within %s, reconnecting",
		tr.resolver.Info.DescriptiveName(),
		defaultRequestTimeout,
	)
	_ = trc.conn.Close()
}
