//go:build android

package network

import (
	"sync/atomic"
	"time"
)

// flowAlive is the TUN flow table. Android cannot read the kernel socket tables.
var flowAlive atomic.Value // func(id string) bool

// SetFlowAlive registers the function that reports whether a connection's flow
// is still proxied. Called from the Android interception stack.
func SetFlowAlive(fn func(id string) bool) {
	if fn == nil {
		return
	}
	flowAlive.Store(fn)
}

// connectionStillActive reports whether atun is still proxying this connection.
// A flow that has left the table is ended, so rule changes can reach the ones
// that are still open.
func connectionStillActive(conn *Connection, _ time.Time) bool {
	if conn == nil {
		return false
	}
	fn, _ := flowAlive.Load().(func(string) bool)
	if fn == nil {
		return false
	}
	return fn(conn.ID)
}
