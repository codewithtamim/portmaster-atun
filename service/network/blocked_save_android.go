//go:build android

package network

import (
	"strconv"
	"sync"
	"time"

	"github.com/safing/portmaster/service/network/packet"
)

// repeatedBlockWindow is how long identical blocked attempts are recorded
// only once.
const repeatedBlockWindow = 2 * time.Second

var (
	recentBlocksLock  sync.Mutex
	recentBlocks      = make(map[string]int64) // key -> unix nanos of the recorded attempt
	recentBlocksSwept int64
)

// isRefused reports whether atun refused the connection. atun answers a
// blocked TCP flow with a reset and a blocked UDP flow with an ICMP error and
// closes it at once, so a refused connection has already ended.
func isRefused(conn *Connection) bool {
	if conn.Type != IPConnection || conn.Inbound {
		return false
	}
	switch conn.IPProtocol { //nolint:exhaustive
	case packet.TCP, packet.UDP:
	default:
		return false
	}
	switch conn.Verdict { //nolint:exhaustive
	case VerdictBlock, VerdictDrop, VerdictFailed:
		return true
	default:
		return false
	}
}

// endRefused marks a refused connection as ended before it is saved.
//
// Otherwise its end reached netquery as a second update, sent when the cleaner
// noticed the flow was gone. netquery drops updates when its feed is full, and
// an app retrying a blocked server hundreds of times a second fills it: the
// lost updates left thousands of rows active, counted as live connections and
// left out of History.
func endRefused(conn *Connection) {
	if conn.Ended == 0 && isRefused(conn) {
		conn.Ended = time.Now().Unix()
	}
}

// skipRepeatedBlock reports whether a refused connection repeats one that was
// recorded moments ago: same app, same destination, same port. A blocked app
// that retries in a tight loop (seen at over a thousand attempts a second)
// would otherwise write a row per attempt and stall netquery for every app.
// DNS stays exact: its attribution looks up each connection.
func skipRepeatedBlock(conn *Connection) bool {
	if !isRefused(conn) || conn.Entity == nil || conn.Entity.Port == 53 {
		return false
	}

	key := conn.ProcessContext.Source + "/" + conn.ProcessContext.Profile + "|" +
		strconv.Itoa(int(conn.IPProtocol)) + "|" + conn.Entity.IP.String() + "|" +
		strconv.Itoa(int(conn.Entity.Port))
	now := time.Now().UnixNano()

	recentBlocksLock.Lock()
	defer recentBlocksLock.Unlock()

	// Drop stale keys now and then, so the map stays small.
	if now-recentBlocksSwept > int64(10*repeatedBlockWindow) {
		for k, at := range recentBlocks {
			if now-at > int64(repeatedBlockWindow) {
				delete(recentBlocks, k)
			}
		}
		recentBlocksSwept = now
	}

	if at, ok := recentBlocks[key]; ok && now-at < int64(repeatedBlockWindow) {
		return true
	}
	recentBlocks[key] = now
	return false
}
