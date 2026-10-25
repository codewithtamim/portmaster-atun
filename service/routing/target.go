// Package routing owns the routing target: where accepted connections go after
// the firewall has let them through.
//
// The target is an Android-only concept. On every other platform the module is
// inert and never joins the service group, so the SPN keeps being driven
// solely by the spn/enable setting.
package routing

// Target is where accepted traffic goes after filtering.
type Target string

// Routing targets.
const (
	// TargetDirect sends accepted traffic out a protected socket.
	TargetDirect Target = "direct"
	// TargetSPN sends accepted traffic through the SPN.
	TargetSPN Target = "spn"
	// TargetWireGuard sends accepted traffic through the userspace WireGuard device.
	TargetWireGuard Target = "wireguard"
)

// Config keys.
const (
	// CfgOptionTargetKey is the routing target setting. Registered on Android only.
	CfgOptionTargetKey = "routing/target"
)

// Targets lists every routing target in UI order.
var Targets = [...]Target{TargetDirect, TargetSPN, TargetWireGuard}

// Valid reports whether t is a known routing target.
func (t Target) Valid() bool {
	switch t {
	case TargetDirect, TargetSPN, TargetWireGuard:
		return true
	default:
		return false
	}
}

// String returns the target as it is stored in the config.
func (t Target) String() string {
	return string(t)
}
