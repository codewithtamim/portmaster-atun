//go:build android

package routing

import (
	"errors"
	"sync/atomic"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/mgr"
)

// Errors.
var (
	// ErrInvalidTarget is returned when an unknown routing target is requested.
	ErrInvalidTarget = errors.New("routing: target must be direct, spn or wireguard")

	errOnlyOneInstance = errors.New("only one instance allowed")
)

// Routing owns the routing target and keeps the SPN in step with it.
type Routing struct {
	mgr      *mgr.Manager
	instance instance

	// EventTargetChanged fires after the target settled. Subscribers must not
	// write config from the callback; reconcile has already done that.
	EventTargetChanged *mgr.EventMgr[Target]
}

// Manager returns the module manager.
func (r *Routing) Manager() *mgr.Manager { return r.mgr }

// Start starts the module.
func (r *Routing) Start() error {
	// Settle the stored target against spn/enable before anything reads it.
	r.migrate()
	r.reconcile()

	r.instance.Config().EventConfigChange.AddCallback(
		"reconcile routing target",
		func(_ *mgr.WorkerCtx, _ struct{}) (bool, error) {
			r.reconcile()
			return false, nil
		},
	)

	return nil
}

// Stop stops the module.
func (r *Routing) Stop() error { return nil }

// prep registers the config option and the API endpoint. Called by New.
func (r *Routing) prep() error {
	if err := registerConfig(); err != nil {
		return err
	}
	return r.registerAPIEndpoints()
}

// Current reports the routing target.
func Current() Target {
	t := Target(config.Concurrent.GetAsString(CfgOptionTargetKey, string(TargetDirect))())
	if !t.Valid() {
		return TargetDirect
	}
	return t
}

// SetTarget stores the routing target.
func SetTarget(target Target) error {
	if !target.Valid() {
		return ErrInvalidTarget
	}
	return config.SetConfigOption(CfgOptionTargetKey, string(target))
}

var (
	module     *Routing
	shimLoaded atomic.Bool
)

// New returns a new routing module.
func New(instance instance) (*Routing, error) {
	if !shimLoaded.CompareAndSwap(false, true) {
		return nil, errOnlyOneInstance
	}
	m := mgr.New("Routing")
	module = &Routing{
		mgr:                m,
		instance:           instance,
		EventTargetChanged: mgr.NewEventMgr[Target]("target changed", m),
	}

	if err := module.prep(); err != nil {
		return nil, err
	}

	return module, nil
}

// migrate adopts the pre-existing spn/enable setting the first time the app runs
// with a routing target. Without this an SPN user would silently drop to direct.
func (r *Routing) migrate() {
	// GetAsString returns the default "direct" when the user has never chosen
	// a target, so a non-empty check never migrates and the first reconcile
	// turns SPN off.
	opt, err := config.GetOption(CfgOptionTargetKey)
	if err != nil {
		log.Warningf("routing: failed to read routing target: %s", err)
		return
	}
	if opt.IsSetByUser() {
		return
	}
	if !config.GetAsBool(cfgOptionEnableSPNKey, false)() {
		return
	}
	if err := config.SetConfigOption(CfgOptionTargetKey, string(TargetSPN)); err != nil {
		log.Warningf("routing: failed to migrate spn/enable to a routing target: %s", err)
		return
	}
	log.Info("routing: migrated enabled SPN to the spn routing target")
}

type instance interface {
	API() *api.API
	Config() *config.Config
}
