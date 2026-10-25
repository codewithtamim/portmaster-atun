// Package wireguard runs a userspace WireGuard device as a routing target.
// Android only. The supervisor worker is the only goroutine that starts or stops the device.
package wireguard

import "errors"

// Errors.
var (
	// ErrNoProfile is returned when no profile is installed.
	ErrNoProfile = errors.New("wireguard: no profile installed")
	// ErrNotConnected is returned when the device cannot carry traffic.
	ErrNotConnected = errors.New("wireguard: tunnel is not connected")
	// ErrBadProfile is returned when a profile fails validation.
	ErrBadProfile = errors.New("wireguard: invalid profile")
	// ErrSplitTunnel is returned, wrapped in ErrBadProfile, for a profile
	// that routes only some addresses. Split tunneling is not in v0.1.
	ErrSplitTunnel = errors.New("config routes only some addresses; split tunneling is not supported")
	// ErrUnsupported is returned on platforms without a WireGuard device.
	ErrUnsupported = errors.New("wireguard: not supported on this platform")
	// ErrResolve is returned when no peer endpoint resolved.
	ErrResolve = errors.New("wireguard: endpoint did not resolve")
	// ErrNoProtector is returned when the VPN service has not registered a
	// socket protector yet. Coming up without one would route the tunnel's own
	// UDP socket back into the TUN.
	ErrNoProtector = errors.New("wireguard: socket protector not registered")
	// ErrProtect is returned when a tunnel socket could not be excluded from the TUN.
	ErrProtect = errors.New("wireguard: failed to protect tunnel socket")
)
