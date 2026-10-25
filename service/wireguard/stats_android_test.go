//go:build android

package wireguard

import (
	"testing"
	"time"
)

func TestParseStats(t *testing.T) {
	t.Parallel()

	// Two peers, as device.IpcGet reports them. The private key is in the same
	// blob and must not end up anywhere in the result.
	const uapi = `private_key=e8a1bd2c00000000000000000000000000000000000000000000000000000000
listen_port=51820
public_key=1111111111111111111111111111111111111111111111111111111111111111
last_handshake_time_sec=1700000000
last_handshake_time_nsec=500
rx_bytes=100
tx_bytes=200
public_key=2222222222222222222222222222222222222222222222222222222222222222
last_handshake_time_sec=1600000000
last_handshake_time_nsec=900
rx_bytes=50
tx_bytes=25
`

	stats := parseStats(uapi)

	if stats.RxBytes != 150 {
		t.Errorf("RxBytes = %d, want 150", stats.RxBytes)
	}
	if stats.TxBytes != 225 {
		t.Errorf("TxBytes = %d, want 225", stats.TxBytes)
	}

	// The newest handshake across peers wins, with its own nsec.
	want := time.Unix(1700000000, 500)
	if !stats.LastHandshake.Equal(want) {
		t.Errorf("LastHandshake = %s, want %s", stats.LastHandshake, want)
	}
}

func TestParseStatsNoHandshake(t *testing.T) {
	t.Parallel()

	stats := parseStats("public_key=abc\nrx_bytes=0\ntx_bytes=0\n")

	if !stats.LastHandshake.IsZero() {
		t.Errorf("LastHandshake = %s, want zero", stats.LastHandshake)
	}
}

func TestParseStatsGarbage(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "\n\n", "no-equals-sign", "rx_bytes=", "rx_bytes=nope"} {
		stats := parseStats(in)
		if stats.RxBytes != 0 || stats.TxBytes != 0 || !stats.LastHandshake.IsZero() {
			t.Errorf("parseStats(%q) = %+v, want zero value", in, stats)
		}
	}
}
