//go:build android

package resolver

import (
	"context"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/stretchr/testify/require"
	"github.com/tevino/abool"
)

func TestLockDialSharesTheNewConnection(t *testing.T) {
	t.Parallel()

	tr := &TCPResolver{}
	ctx := context.Background()

	conn, unlock, err := tr.lockDial(ctx, 0)
	require.NoError(t, err)
	require.Nil(t, conn)

	// A second query waits while the first one dials.
	got := make(chan *tcpResolverConn, 1)
	go func() {
		c, u, e := tr.lockDial(ctx, 0)
		if e == nil && c == nil {
			u()
		}
		got <- c
	}()
	select {
	case <-got:
		t.Fatal("second query did not wait for the dial in progress")
	case <-time.After(50 * time.Millisecond):
	}

	// The first query connects and releases the slot.
	created := &tcpResolverConn{abandoned: abool.New()}
	tr.Lock()
	tr.resolverConnInstanceID++
	tr.resolverConn = created
	tr.Unlock()
	unlock()

	select {
	case c := <-got:
		require.Same(t, created, c)
	case <-time.After(time.Second):
		t.Fatal("second query did not get the slot")
	}
}

func TestLockDialRetriesAfterAFailedDial(t *testing.T) {
	t.Parallel()

	tr := &TCPResolver{}
	ctx := context.Background()

	_, unlock, err := tr.lockDial(ctx, 0)
	require.NoError(t, err)
	unlock() // The dial failed: no new connection.

	conn, unlock, err := tr.lockDial(ctx, 0)
	require.NoError(t, err)
	require.Nil(t, conn, "the next query must dial itself")
	unlock()
}

func TestLockDialStopsWaitingWhenTheQueryEnds(t *testing.T) {
	t.Parallel()

	tr := &TCPResolver{}
	_, unlock, err := tr.lockDial(context.Background(), 0)
	require.NoError(t, err)
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	conn, _, err := tr.lockDial(ctx, 0)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, conn)
}

// The package tests run a live resolver module, which adds slots of its own,
// so this checks the key of one resolver rather than counting.
func TestDialSlotIsDroppedWithItsResolver(t *testing.T) {
	t.Parallel()

	var key weak.Pointer[TCPResolver]
	func() {
		tr := &TCPResolver{}
		key = weak.Make(tr)
		_, unlock, err := tr.lockDial(context.Background(), 0)
		require.NoError(t, err)
		unlock()
		_, ok := dialSlots.Load(key)
		require.True(t, ok)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := dialSlots.Load(key); !ok {
			return
		}
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("slot of a collected resolver was not deleted")
}

func TestDialSlotIsStablePerResolver(t *testing.T) {
	t.Parallel()

	tr := &TCPResolver{}
	require.Equal(t, dialSlot(tr), dialSlot(tr))
	require.NotEqual(t, dialSlot(tr), dialSlot(&TCPResolver{}))
}
