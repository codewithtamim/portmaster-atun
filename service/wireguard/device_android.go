//go:build android

package wireguard

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aprotect"
)

// session is one running device. Only the supervisor touches it, so nothing in
// here is locked.
type session struct {
	dev  *device.Device
	net  *netstack.Net
	bind conn.Bind

	profile   *Profile
	mtu       int
	resolved  map[int]netip.AddrPort
	endpoints []netip.Addr

	startedAt      time.Time
	everHandshaked bool
	lastRx, lastTx uint64
	staleSince     time.Time

	// probe tracks whether IPv6 actually gets through. See probe_android.go.
	probe probeState
}

// startSession brings a device up for the profile.
func startSession(ctx context.Context, profile *Profile) (*session, error) {
	// Coming up without a protector would put the tunnel's own UDP socket inside
	// the TUN, so its handshake would travel through itself. aprotect lets
	// unprotected sockets through before the VPN owns the default route, which is
	// right for boot-time DNS but wrong for us.
	if !aprotect.HasProtector() {
		return nil, ErrNoProtector
	}

	resolved, err := resolveEndpoints(ctx, profile)
	if err != nil {
		return nil, err
	}

	// DNS is deliberately nil: Portmaster keeps owning name resolution.
	mtu := tunnelMTU()
	tunDev, netStack, err := netstack.CreateNetTUN(profile.InterfaceAddrs(), nil, mtu)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create netstack: %w", err)
	}

	bind, err := newProtectedBind()
	if err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	dev := device.NewDevice(tunDev, bind, &device.Logger{
		Verbosef: func(format string, args ...any) {
			log.Tracef("wireguard: %s", redact(format, args...))
		},
		Errorf: func(format string, args ...any) {
			log.Warningf("wireguard: %s", redact(format, args...))
		},
	})
	dev.DisableSomeRoamingForBrokenMobileSemantics()

	if err := dev.IpcSet(profile.UAPI(resolved)); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wireguard: apply config: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wireguard: device up: %w", err)
	}

	s := &session{
		dev:       dev,
		net:       netStack,
		bind:      bind,
		profile:   profile,
		mtu:       mtu,
		resolved:  resolved,
		endpoints: endpointAddrs(resolved),
		startedAt: time.Now(),
	}

	log.Infof("wireguard: device up for %q via %s, mtu %d", profile.Name, s.endpointString(), mtu)
	return s, nil
}

func (s *session) close() {
	if s == nil || s.dev == nil {
		return
	}
	s.dev.Close()
}

// rebind re-resolves the endpoints and rebuilds the UDP sockets.
//
// Called when the network changed or the peer went quiet. BindUpdate creates new
// sockets, so they have to be protected again - forgetting that leaves the
// tunnel talking through an unprotected socket that loops back into the TUN.
func (s *session) rebind(ctx context.Context) error {
	resolved, err := resolveEndpoints(ctx, s.profile)
	if err != nil {
		return err
	}

	// New sockets are protected inside Open, before any handshake is sent.
	if err := s.dev.BindUpdate(); err != nil {
		return fmt.Errorf("wireguard: rebind: %w", err)
	}
	if err := s.dev.IpcSet(s.profile.EndpointUAPI(resolved)); err != nil {
		return fmt.Errorf("wireguard: reapply endpoint: %w", err)
	}

	s.resolved = resolved
	s.endpoints = endpointAddrs(resolved)
	// The path changed, so what the probe learned may no longer hold.
	s.resetProbe()

	log.Infof("wireguard: rebound to %s", s.endpointString())
	return nil
}

func (s *session) stats() Stats {
	uapi, err := s.dev.IpcGet()
	if err != nil {
		return Stats{}
	}
	return parseStats(uapi)
}

func peerHost(profile *Profile) string {
	if profile == nil || len(profile.Peers) == 0 {
		return ""
	}
	return profile.Peers[0].EndpointHost
}

func (s *session) endpointString() string {
	parts := make([]string, 0, len(s.resolved))
	for i := range s.profile.Peers {
		if ap, ok := s.resolved[i]; ok {
			parts = append(parts, ap.String())
		}
	}
	return strings.Join(parts, ", ")
}

func endpointAddrs(resolved map[int]netip.AddrPort) []netip.Addr {
	addrs := make([]netip.Addr, 0, len(resolved))
	for _, ap := range resolved {
		addrs = append(addrs, ap.Addr().Unmap())
	}
	return addrs
}

// redact strips key material out of a wireguard-go log line. The device logs the
// UAPI blob on a config error, and that blob holds the private key.
func redact(format string, args ...any) string {
	msg := fmt.Sprintf(format, args...)
	for _, marker := range []string{"private_key=", "preshared_key="} {
		if i := strings.Index(msg, marker); i >= 0 {
			msg = msg[:i+len(marker)] + "[redacted]"
		}
	}
	return msg
}
