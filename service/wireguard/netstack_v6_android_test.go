//go:build android

package wireguard

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/safing/portmaster/service/mgr"
)

// testPeer is one side of an in-process WireGuard pair.
type testPeer struct {
	dev  *device.Device
	net  *netstack.Net
	port uint16
}

// newTestPeer brings up a netstack device the way startSession does, but on a
// plain loopback bind, so the pair can talk without a phone or a real server.
func newTestPeer(t *testing.T, addrs []netip.Addr, mtu int, privHex string) *testPeer {
	t.Helper()

	tunDev, netStack, err := netstack.CreateNetTUN(addrs, nil, mtu)
	if err != nil {
		t.Fatalf("create netstack: %s", err)
	}
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "test: "))
	t.Cleanup(dev.Close)

	if err := dev.IpcSet("private_key=" + privHex + "\nlisten_port=0\n"); err != nil {
		t.Fatalf("set key: %s", err)
	}
	if err := dev.Up(); err != nil {
		t.Fatalf("device up: %s", err)
	}

	uapi, err := dev.IpcGet()
	if err != nil {
		t.Fatalf("ipc get: %s", err)
	}
	var port uint16
	for line := range strings.SplitSeq(uapi, "\n") {
		if _, scanErr := fmt.Sscanf(line, "listen_port=%d", &port); scanErr == nil {
			break
		}
	}
	if port == 0 {
		t.Fatalf("no listen port in uapi")
	}

	return &testPeer{dev: dev, net: netStack, port: port}
}

// addPeer configures a peer. A port of 0 leaves the endpoint unset, so this
// side only answers, like a server that learns the client from its handshake.
// Both sides initiating at once is a handshake race that stalls the test.
func (p *testPeer) addPeer(t *testing.T, pubHex string, port uint16, allowed ...string) {
	t.Helper()

	cfg := "public_key=" + pubHex + "\n"
	for _, a := range allowed {
		cfg += "allowed_ip=" + a + "\n"
	}
	if port != 0 {
		cfg += fmt.Sprintf("endpoint=127.0.0.1:%d\npersistent_keepalive_interval=1\n", port)
	}
	if err := p.dev.IpcSet(cfg); err != nil {
		t.Fatalf("add peer: %s", err)
	}
}

// Fixed curve25519 key pairs, so the test needs no key generation.
const (
	clientPriv = "1011111111111111111111111111111111111111111111111111111111111151"
	clientPub  = "7b4e909bbe7ffe44c465a220037d608ee35897d31ef972f07f74892cb0f73f13"
	serverPriv = "2022222222222222222222222222222222222222222222222222222222222262"
	serverPub  = "0faa684ed28867b97f4a6a2dee5df8ce974e76b7018e3f22a1c4cf2678570f20"
)

// TestNetstackTransfersBothFamilies moves a body larger than the MTU through
// the tunnel in each direction, over IPv4 and over IPv6. A v6 connection has
// been seen to complete the handshake and then never receive a byte.
func TestNetstackTransfersBothFamilies(t *testing.T) {
	for _, h := range []string{clientPriv, clientPub, serverPriv, serverPub} {
		if _, err := hex.DecodeString(h); err != nil {
			t.Fatalf("bad test key: %s", err)
		}
	}

	serverV4 := netip.MustParseAddr("10.64.0.1")
	serverV6 := netip.MustParseAddr("fc00:bbbb:bbbb:bb01::1")
	clientV4 := netip.MustParseAddr("10.64.0.2")
	clientV6 := netip.MustParseAddr("fc00:bbbb:bbbb:bb01::2")

	server := newTestPeer(t, []netip.Addr{serverV4, serverV6}, maxTunnelMTU, serverPriv)
	client := newTestPeer(t, []netip.Addr{clientV4, clientV6}, maxTunnelMTU, clientPriv)
	server.addPeer(t, clientPub, 0, clientV4.String()+"/32", clientV6.String()+"/128")
	client.addPeer(t, serverPub, server.port, "0.0.0.0/0", "::/0")

	payload := bytes.Repeat([]byte("portmaster-ipv6-"), 16<<10) // 256 KiB, many segments.

	for _, target := range []netip.Addr{serverV4, serverV6} {
		t.Run(target.String(), func(t *testing.T) {
			ln, err := server.net.ListenTCPAddrPort(netip.AddrPortFrom(target, 8080))
			if err != nil {
				t.Fatalf("listen: %s", err)
			}
			defer func() { _ = ln.Close() }()

			// Echo server: read the whole upload, then send it back.
			serverErr := make(chan error, 1)
			go func() {
				c, err := ln.Accept()
				if err != nil {
					serverErr <- err
					return
				}
				defer func() { _ = c.Close() }()
				buf := make([]byte, len(payload))
				if _, err := io.ReadFull(c, buf); err != nil {
					serverErr <- fmt.Errorf("server read: %w", err)
					return
				}
				_, err = c.Write(buf)
				serverErr <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			c, err := client.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(target, 8080))
			if err != nil {
				t.Fatalf("dial: %s", err)
			}
			defer func() { _ = c.Close() }()
			_ = c.SetDeadline(time.Now().Add(20 * time.Second))

			if _, err := c.Write(payload); err != nil {
				t.Fatalf("client write: %s", err)
			}
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(c, got); err != nil {
				t.Fatalf("client read: %s", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("echo mismatch")
			}
			if err := <-serverErr; err != nil {
				t.Fatalf("server: %s", err)
			}
		})
	}
}

// TestProbeV6 checks the probe against a server that carries both families and
// one that, like IVPN jp-tok-jp2, completes the handshake but drops
// inner IPv6. The drop is reproduced with cryptokey routing: the server only
// accepts the client's IPv4 source address.
func TestProbeV6(t *testing.T) {
	serverV4 := netip.MustParseAddr("10.64.0.1")
	serverV6 := netip.MustParseAddr("fc00:bbbb:bbbb:bb01::1")
	clientV4 := netip.MustParseAddr("10.64.0.2")
	clientV6 := netip.MustParseAddr("fc00:bbbb:bbbb:bb01::2")
	targetV4 := []netip.AddrPort{netip.AddrPortFrom(serverV4, 443)}
	targetV6 := []netip.AddrPort{netip.AddrPortFrom(serverV6, 443)}

	cases := []struct {
		name          string
		serverAllowed []string
		serverListens bool
		want          v6Reach
	}{
		{"both families", []string{clientV4.String() + "/32", clientV6.String() + "/128"}, true, v6Usable},
		{"server drops ipv6", []string{clientV4.String() + "/32"}, true, v6Broken},
		// Nothing answers on either family: the tunnel is the problem, so the
		// probe must not single out IPv6.
		{"nothing answers", []string{clientV4.String() + "/32"}, false, v6Unknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestPeer(t, []netip.Addr{serverV4, serverV6}, maxTunnelMTU, serverPriv)
			client := newTestPeer(t, []netip.Addr{clientV4, clientV6}, maxTunnelMTU, clientPriv)
			server.addPeer(t, clientPub, 0, tc.serverAllowed...)
			client.addPeer(t, serverPub, server.port, "0.0.0.0/0", "::/0")

			if tc.serverListens {
				for _, target := range append(append([]netip.AddrPort{}, targetV4...), targetV6...) {
					ln, err := server.net.ListenTCPAddrPort(target)
					if err != nil {
						t.Fatalf("listen %s: %s", target, err)
					}
					t.Cleanup(func() { _ = ln.Close() })
				}
			}

			m := mgr.New("wireguard probe test")
			got := probeV6(context.Background(), m, client.net, targetV6, targetV4)
			if got != tc.want {
				t.Fatalf("probeV6 = %d, want %d", got, tc.want)
			}
		})
	}
}
