//go:build android

package network

import (
	"runtime"
	"sort"
	"time"
)

// Ended connections normally stay in memory for ten minutes. A flood (an app
// retrying a blocked server, a burst of blocked lookups) then holds every
// record of those minutes at once, hundreds of megabytes on a phone. Above
// maxEndedConnsKept per store the oldest ended records are dropped early.
// Nothing is lost for the user: history on disk and the live database have
// them already.
const (
	maxEndedConnsKept = 2000

	// Records that ended moments ago may still be looked up while their flow
	// winds down, so they are never trimmed. A lookup is over once answered.
	trimMinEndedAge    = 10 * time.Second
	trimMinEndedAgeDNS = 3 * time.Second

	// After this many records are dropped in one pass a collection is run.
	// The GC paces itself by the heap of its last cycle, which during a flood
	// is large; once the flood stops nothing allocates, so the freed records
	// would otherwise stay as garbage until the two-minute forced cycle.
	collectAfterTrimming = 1000
)

type endedConn struct {
	conn *Connection
	at   int64
}

func trimEndedConnections(now time.Time) {
	dropped := trimEndedIn(conns, now.Add(-trimMinEndedAge).Unix()) +
		trimEndedIn(dnsConns, now.Add(-trimMinEndedAgeDNS).Unix())
	if dropped >= collectAfterTrimming {
		go runtime.GC()
	}
}

// trimEndedIn returns how many records it dropped.
func trimEndedIn(cs *connectionStore, endedBefore int64) int {
	// Cheap check first: this runs on every cleaner tick.
	if cs.len() <= maxEndedConnsKept {
		return 0
	}
	ended := make([]endedConn, 0, cs.len())
	for _, conn := range cs.list() {
		conn.Lock()
		at := conn.Ended
		conn.Unlock()
		if at != 0 && at < endedBefore {
			ended = append(ended, endedConn{conn, at})
		}
	}
	dropped := 0
	for _, e := range oldestBeyond(ended, maxEndedConnsKept) {
		e.conn.Lock()
		// Unchanged since it was listed: not reopened or deleted meanwhile.
		if e.conn.Ended == e.at {
			// Unlike the cleaner's delete, no update is pushed: netquery
			// would save the record once more, to memory and to disk, which
			// during a flood doubles the writes for no change in the data.
			cs.delete(e.conn)
			e.conn.Meta().Delete()
			dropped++
		}
		e.conn.Unlock()
	}
	return dropped
}

// oldestBeyond returns the entries to drop so that the keep newest remain.
func oldestBeyond(ended []endedConn, keep int) []endedConn {
	if len(ended) <= keep {
		return nil
	}
	sort.Slice(ended, func(i, j int) bool { return ended[i].at < ended[j].at })
	return ended[:len(ended)-keep]
}
