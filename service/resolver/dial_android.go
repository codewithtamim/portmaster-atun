//go:build android

package resolver

import (
	"context"
	"runtime"
	"sync"
	"weak"
)

// dialSlots holds one dial slot per TCP resolver. The key is weak and the
// entry is deleted once its resolver is collected, so resolvers replaced on a
// network change do not pile up here.
var dialSlots sync.Map // weak.Pointer[TCPResolver] -> chan struct{}

func dialSlot(tr *TCPResolver) chan struct{} {
	key := weak.Make(tr)
	value, loaded := dialSlots.LoadOrStore(key, make(chan struct{}, 1))
	if !loaded {
		runtime.AddCleanup(tr, func(k weak.Pointer[TCPResolver]) {
			dialSlots.Delete(k)
		}, key)
	}
	return value.(chan struct{}) //nolint:forcetypeassert // Only this function stores values.
}

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
	slot := dialSlot(tr)

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
