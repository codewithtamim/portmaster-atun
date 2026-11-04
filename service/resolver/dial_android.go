//go:build android

package resolver

import (
	"context"
	"sync"
)

// dialSlots holds one dial slot per TCP resolver.
var dialSlots sync.Map // *TCPResolver -> chan struct{}

// lockDial lets one query at a time connect to the resolver.
//
// Without it, every query that finds no connection dials its own. Through the
// SPN each dial is a new tunnel and TLS handshake, and a burst of lookups
// started dozens at once: they slowed each other past the connect timeout,
// so the lookups failed and the next burst dialed again. Browsing over SPN
// opened over 200 connections to the resolver in ten minutes.
//
// A query that waited for the slot uses the connection the previous holder
// created, if there is one. Otherwise it dials and must call unlock after.
func (tr *TCPResolver) lockDial(ctx context.Context, seenInstanceID int) (conn *tcpResolverConn, unlock func(), err error) {
	value, _ := dialSlots.LoadOrStore(tr, make(chan struct{}, 1))
	slot := value.(chan struct{}) //nolint:forcetypeassert // Only this function stores values.

	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	tr.Lock()
	newConn := tr.resolverConn
	newInstance := tr.resolverConnInstanceID != seenInstanceID
	tr.Unlock()
	if newInstance && newConn != nil && newConn.abandoned.IsNotSet() {
		<-slot
		return newConn, nil, nil
	}

	return nil, func() { <-slot }, nil
}
