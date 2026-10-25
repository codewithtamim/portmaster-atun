//go:build !android

package wireguard

import (
	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/netenv"
)

// WireGuard is the Android routing target's device. service.New still
// constructs it on other platforms so instance.go stays shared, but it never
// runs there: its nil manager makes mgr.Group skip it.
type WireGuard struct{}

// New returns the inert desktop module.
func New(_ instance) (*WireGuard, error) {
	return &WireGuard{}, nil
}

// Manager returns nil, which keeps this module out of the service group.
func (w *WireGuard) Manager() *mgr.Manager { return nil }

// Start is never called on this platform.
func (w *WireGuard) Start() error { return nil }

// Stop is never called on this platform.
func (w *WireGuard) Stop() error { return nil }

// InstallProfile is not supported here.
func InstallProfile(_ string) error { return ErrUnsupported }

// ClearProfile is not supported here.
func ClearProfile() error { return ErrUnsupported }

type instance interface {
	API() *api.API
	Config() *config.Config
	NetEnv() *netenv.NetEnv
}
