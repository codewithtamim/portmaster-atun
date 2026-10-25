//go:build android

package wireguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"golang.zx2c4.com/wireguard/tun/netstack"
)

var errUnsupportedDialNetwork = errors.New("wireguard: unsupported network")

// DialContext dials through the tunnel.
//
// atun has already terminated the app's flow and the firewall has already
// decided; this is where an accepted connection actually enters the tunnel. The
// address is always a literal IP, because Portmaster resolved the name itself.
//
// It fails closed: if the tunnel cannot carry the connection right now, the dial
// fails rather than falling back to a direct socket.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if module == nil {
		return nil, ErrUnsupported
	}

	netStack, err := module.dialStack()
	if err != nil {
		return nil, err
	}

	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("wireguard: dial address %q: %w", address, err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("wireguard: dial port %q: %w", portStr, err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("wireguard: dial host %q: %w", host, err)
	}
	target := netip.AddrPortFrom(addr.Unmap(), uint16(port))

	switch network {
	case "tcp", "tcp4", "tcp6":
		conn, err := netStack.DialContextTCPAddrPort(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("wireguard: dial tcp %s: %w", target, err)
		}
		return conn, nil

	case "udp", "udp4", "udp6":
		conn, err := netStack.DialUDPAddrPort(netip.AddrPort{}, target)
		if err != nil {
			return nil, fmt.Errorf("wireguard: dial udp %s: %w", target, err)
		}
		return conn, nil

	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedDialNetwork, network)
	}
}

// dialStack returns the network stack to dial through, if the tunnel is up.
//
// liveStack is published by the supervisor and read here with a plain atomic
// load: this runs once per accepted connection, so it must not contend with the
// supervisor or with other dials.
func (w *WireGuard) dialStack() (*netstack.Net, error) {
	if Status().State != StateConnected {
		return nil, ErrNotConnected
	}

	stack := w.liveStack.Load()
	if stack == nil {
		return nil, ErrNotConnected
	}
	return stack, nil
}
