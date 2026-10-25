//go:build android

package wireguard

import (
	"strconv"
	"strings"
	"time"
)

// Stats is what a device reports about itself, summed over its peers.
type Stats struct {
	RxBytes       uint64
	TxBytes       uint64
	LastHandshake time.Time
}

// parseStats reads the output of device.IpcGet.
//
// The format is "key=value" lines; a public_key line starts a new peer. Only the
// counters are read here - the private key is in the same blob, so nothing in
// this function may log or return anything it does not explicitly pick out.
func parseStats(uapi string) Stats {
	var stats Stats
	var handshakeSec, handshakeNsec int64

	for line := range strings.SplitSeq(uapi, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}

		switch key {
		case "rx_bytes":
			stats.RxBytes += parseUint(value)
		case "tx_bytes":
			stats.TxBytes += parseUint(value)
		case "last_handshake_time_sec":
			if sec := parseInt(value); sec > handshakeSec {
				handshakeSec, handshakeNsec = sec, 0
			}
		case "last_handshake_time_nsec":
			// Belongs to the peer whose sec line we just took as the newest.
			if handshakeSec > 0 && handshakeNsec == 0 {
				handshakeNsec = parseInt(value)
			}
		}
	}

	if handshakeSec > 0 {
		stats.LastHandshake = time.Unix(handshakeSec, handshakeNsec)
	}

	return stats
}

// parseUint parses s as an unsigned integer, returning 0 on any parse
// failure. A zero is indistinguishable from a missing/zero field in the
// device's UAPI output, which is the desired behaviour: malformed values
// simply do not contribute to the counters.
//
// Nothing here may log or return what it did not explicitly pick out: the
// same blob carries the private key, so parse failures are swallowed on
// purpose (see parseStats).
func parseUint(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseInt parses s as a signed integer, returning 0 on any parse failure.
// See parseUint for why parse failures are swallowed on purpose.
func parseInt(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
