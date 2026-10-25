//go:build !android

package routing

import (
	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/service/mgr"
)

// Routing is the Android routing target. service.New still constructs it on
// other platforms so instance.go stays shared, but it never runs there: its
// nil manager makes mgr.Group skip it, so it is neither started nor listed.
type Routing struct{}

// New returns the inert desktop module.
func New(_ instance) (*Routing, error) {
	return &Routing{}, nil
}

// Manager returns nil, which keeps this module out of the service group.
func (r *Routing) Manager() *mgr.Manager { return nil }

// Start is never called on this platform.
func (r *Routing) Start() error { return nil }

// Stop is never called on this platform.
func (r *Routing) Stop() error { return nil }

type instance interface {
	API() *api.API
	Config() *config.Config
}
