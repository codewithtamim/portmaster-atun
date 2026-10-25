//go:build android

package wireguard

import (
	"net"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/service/intel/geoip"
)

// State is where the device is in its lifecycle.
type State uint8

// Device states.
const (
	// StateDown means no device is running.
	StateDown State = iota
	// StateConnecting means the device is up but has not handshaked yet.
	StateConnecting
	// StateConnected means the tunnel can carry traffic.
	StateConnected
	// StateDegraded means the tunnel handshaked once but the peer stopped
	// answering. Traffic is blocked while we try to recover.
	StateDegraded
	// StateFailed means bring-up failed and we are backing off.
	StateFailed
)

// String returns the state as a stable, machine-readable name. The app switches
// on these, so they are API.
func (s State) String() string {
	switch s {
	case StateDown:
		return "down"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateDegraded:
		return "degraded"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Route is what to do with a connection while WireGuard is the routing target.
type Route uint8

// Routes.
const (
	// RouteBlock blocks the connection. The tunnel cannot carry it and letting it
	// out directly would leak it.
	RouteBlock Route = iota
	// RouteTunnel sends the connection through the tunnel.
	RouteTunnel
	// RouteBlockFamily blocks the connection because the connected tunnel does
	// not carry its address family, either by config or because the server
	// drops it (see probe_android.go). Blocking lets apps fall back to the
	// other family at once instead of waiting on a blackhole.
	RouteBlockFamily
)

// Snapshot is an immutable view of the device.
//
// Readers take it with a single atomic load, so the connection path never waits
// on the supervisor and the supervisor never waits on a reader. Nothing in here
// is a secret; the whole struct is published to the app.
type Snapshot struct {
	State         State     `json:"state"`
	Profile       Meta      `json:"profile"`
	Endpoint      string    `json:"endpoint"`
	LastHandshake time.Time `json:"lastHandshake"`
	RxBytes       uint64    `json:"rxBytes"`
	TxBytes       uint64    `json:"txBytes"`
	ErrCode       string    `json:"errCode"`
	ErrDetail     string    `json:"errDetail"`

	// endpoints are the resolved peer endpoints. Traffic to these must not be
	// tunnelled or the handshake would try to travel through itself.
	endpoints []netip.Addr
	// peerHost is the configured endpoint hostname, kept off the API payload.
	peerHost string
}

// Error codes. Stable; the app maps them to localized strings.
const (
	ErrCodeNone       = ""
	ErrCodeResolve    = "resolve"
	ErrCodeDeviceUp   = "device_up"
	ErrCodeConfig     = "config"
	ErrCodeNoProtect  = "no_protector"
	ErrCodeHandshake  = "handshake"
	ErrCodeNoProfileC = "no_profile"
)

// downSnapshot is shared by every reader before the first device ever starts, so
// the atomic pointer is never nil and readers need no nil check.
var downSnapshot = &Snapshot{State: StateDown}

var current atomic.Pointer[Snapshot]

func init() {
	current.Store(downSnapshot)
}

// Status returns the current device state. Safe for concurrent use.
func Status() Snapshot {
	return *current.Load()
}

// ActivePeer is the peer a WireGuard-routed connection should show.
// Country is an ISO code and stays empty when geoip has no answer.
// One atomic load, so the connection path does not wait on the supervisor.
func ActivePeer() (configName, host, address, country string) {
	snap := current.Load()
	configName = snap.Profile.Name
	host = snap.peerHost
	if len(snap.endpoints) == 0 {
		return configName, host, "", ""
	}
	address = snap.endpoints[0].String()
	loc, err := geoip.GetLocation(net.IP(snap.endpoints[0].AsSlice()))
	if err != nil || loc == nil {
		return configName, host, address, ""
	}
	return configName, host, address, loc.Country.Code
}

// store publishes a new snapshot. Only the supervisor calls this.
func store(s *Snapshot) {
	current.Store(s)
}

// RouteFor reports what to do with a connection to ip while WireGuard is the
// routing target.
//
// It fails closed: anything the tunnel cannot carry right now is blocked, never
// sent out directly. A single atomic load, safe on the connection path.
func RouteFor(ip net.IP) Route {
	snapshot := current.Load()
	if snapshot.State != StateConnected {
		return RouteBlock
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return RouteBlock
	}
	addr = addr.Unmap()

	// A family the peers do not route would be dropped inside the device by
	// cryptokey routing. Block it here so it shows up as a decision, not a
	// blackhole.
	v4, v6 := snapshot.Profile.CapturesV4, snapshot.Profile.CapturesV6
	if addr.Is4() {
		if !v4 {
			return RouteBlockFamily
		}
	} else if !v6 {
		return RouteBlockFamily
	}

	return RouteTunnel
}

// IsEndpoint reports whether ip is a resolved peer endpoint. The handshake to it
// rides a protected socket and must never be routed into the tunnel.
func IsEndpoint(ip net.IP) bool {
	snapshot := current.Load()
	if len(snapshot.endpoints) == 0 {
		return false
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()

	return slices.Contains(snapshot.endpoints, addr)
}

// StateView is the wire shape of a Snapshot: what the API endpoint returns, what
// the runtime record carries, and what libpm renders. It holds no key material.
type StateView struct {
	State         string `json:"state"`
	Profile       Meta   `json:"profile"`
	Endpoint      string `json:"endpoint"`
	LastHandshake int64  `json:"lastHandshake"`
	RxBytes       uint64 `json:"rxBytes"`
	TxBytes       uint64 `json:"txBytes"`
	ErrCode       string `json:"errCode"`
	ErrDetail     string `json:"errDetail"`
}

// StateOf renders a snapshot for the wire.
func StateOf(s *Snapshot) StateView {
	view := StateView{
		State:     s.State.String(),
		Profile:   s.Profile,
		Endpoint:  s.Endpoint,
		RxBytes:   s.RxBytes,
		TxBytes:   s.TxBytes,
		ErrCode:   s.ErrCode,
		ErrDetail: s.ErrDetail,
	}
	if !s.LastHandshake.IsZero() {
		view.LastHandshake = s.LastHandshake.Unix()
	}
	return view
}
