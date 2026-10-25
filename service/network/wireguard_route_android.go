//go:build android

package network

// WireGuardRoute is the monitor path for a connection sent through WireGuard.
//
// It is stored in TunnelContext so the existing netquery save writes it into
// extra_data.tunnel. Desktop builds do not include this type, and the shared
// connection record stays unchanged.
//
// Country is an ISO code. The app turns that into a display name.
// StopTunnel does nothing: the WireGuard device is not an SPN tunnel, and
// re-evaluation calls StopTunnel whenever the verdict is not RerouteToTunnel.
type WireGuardRoute struct {
	Kind       string `json:"kind"`
	ConfigName string `json:"configName,omitempty"`
	Country    string `json:"country,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Address    string `json:"address,omitempty"`
}

// GetExitNodeID reports no SPN exit. WireGuard hops are not exit nodes.
func (r *WireGuardRoute) GetExitNodeID() string { return "" }

// StopTunnel does nothing. See WireGuardRoute.
func (r *WireGuardRoute) StopTunnel() error { return nil }
