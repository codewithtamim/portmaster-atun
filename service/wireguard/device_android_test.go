//go:build android

package wireguard

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// handshaked reports whether the device has completed a handshake.
func handshaked(t *testing.T, p *testPeer) bool {
	t.Helper()

	uapi, err := p.dev.IpcGet()
	if err != nil {
		t.Fatalf("ipc get: %s", err)
	}
	for line := range strings.SplitSeq(uapi, "\n") {
		var sec int64
		if _, err := fmt.Sscanf(line, "last_handshake_time_sec=%d", &sec); err == nil && sec != 0 {
			return true
		}
	}
	return false
}

// TestStartHandshakesWithoutKeepalive checks that a peer with no persistent
// keepalive still handshakes as soon as the device is up, with no traffic.
// Without it, check would rebuild an idle tunnel for lack of a handshake.
func TestStartHandshakesWithoutKeepalive(t *testing.T) {
	serverAddr := netip.MustParseAddr("10.64.0.1")
	clientAddr := netip.MustParseAddr("10.64.0.2")

	server := newTestPeer(t, []netip.Addr{serverAddr}, maxTunnelMTU, serverPriv)
	client := newTestPeer(t, []netip.Addr{clientAddr}, maxTunnelMTU, clientPriv)
	server.addPeer(t, clientPub, 0, clientAddr.String()+"/32")

	// The client side as startSession configures it for a profile with no
	// keepalive: an endpoint, and no persistent_keepalive_interval.
	endpoint := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), server.port)
	if err := client.dev.IpcSet(fmt.Sprintf("public_key=%s\nallowed_ip=0.0.0.0/0\nendpoint=%s\n", serverPub, endpoint)); err != nil {
		t.Fatalf("add peer: %s", err)
	}

	time.Sleep(500 * time.Millisecond)
	if handshaked(t, client) {
		t.Fatal("handshake without traffic or a kick; the test cannot tell the kick apart")
	}

	pub, err := hex.DecodeString(serverPub)
	if err != nil {
		t.Fatal(err)
	}
	profile := &Profile{Peers: []Peer{{PublicKey: base64.StdEncoding.EncodeToString(pub)}}}
	startHandshakes(client.dev, profile, map[int]netip.AddrPort{0: endpoint})

	deadline := time.Now().Add(5 * time.Second)
	for !handshaked(t, client) {
		if time.Now().After(deadline) {
			t.Fatal("no handshake after startHandshakes")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
