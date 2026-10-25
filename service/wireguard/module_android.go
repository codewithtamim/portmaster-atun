//go:build android

package wireguard

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/runtime"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/netenv"
	"github.com/safing/portmaster/service/routing"
)

// WireGuard runs the userspace WireGuard device.
// The supervisor worker is the only goroutine that starts or stops it.
type WireGuard struct {
	mgr      *mgr.Manager
	instance instance

	// wake nudges the supervisor. Capacity 1: a pending wake already covers any
	// further change, because the supervisor re-reads everything when it runs.
	wake chan struct{}

	// networkDirty asks the supervisor to rebind. Set from the network watcher,
	// cleared by the supervisor, so the device stays single-owner.
	networkDirty atomic.Bool

	// profileLock guards wanted. Read by the supervisor via wantedProfile.
	profileLock sync.Mutex
	wanted      *Profile

	// liveStack is the stack DialContext dials through. Published by the
	// supervisor, read by every accepted connection, so it is atomic rather than
	// locked. Nil whenever the tunnel is not carrying traffic.
	liveStack atomic.Pointer[netstack.Net]

	// Owned by the supervisor. Never touched from another goroutine.
	session  *session
	failures int
	retryAt  time.Time

	// Runtime record publishing. pushLock guards only the rate limiting, not the
	// state itself, which already lives in the atomic snapshot.
	publishUpdate runtime.PushFunc
	pushLock      sync.Mutex
	lastPushed    pushedState
	lastPushedAt  time.Time
}

// pushedState is the part of a Snapshot that is pushed at once when it
// changes. Changes to anything else, such as the counters, are rate limited.
type pushedState struct {
	State   State
	Profile Meta
	ErrCode string
}

// Manager returns the module manager.
func (w *WireGuard) Manager() *mgr.Manager { return w.mgr }

// Start starts the module.
func (w *WireGuard) Start() error {
	w.mgr.Go("wireguard supervisor", w.supervisor)
	w.mgr.Go("wireguard network watcher", w.networkWatcher)

	// The routing target decides whether the device should run at all.
	w.instance.Routing().EventTargetChanged.AddCallback(
		"wake wireguard on routing target change",
		func(_ *mgr.WorkerCtx, _ routing.Target) (bool, error) {
			w.nudge()
			return false, nil
		},
	)

	return nil
}

// Stop stops the module. The supervisor tears the device down as it exits.
func (w *WireGuard) Stop() error { return nil }

// prep registers the API endpoint and the runtime record. Called by New.
func (w *WireGuard) prep() error {
	return w.registerAPI()
}

// networkWatcher flags a network change so the supervisor rebinds, rather than
// waiting for the peer to time out.
func (w *WireGuard) networkWatcher(ctx *mgr.WorkerCtx) error {
	flag := netenv.GetNetworkChangedFlag()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-flag.Signal():
			flag.Refresh()
			w.networkDirty.Store(true)
			w.nudge()
		}
	}
}

func (w *WireGuard) nudge() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *WireGuard) wantedProfile() *Profile {
	w.profileLock.Lock()
	defer w.profileLock.Unlock()
	return w.wanted
}

// InstallProfile stores a profile and asks the supervisor to apply it.
//
// The document is parsed and validated here so the caller gets a real error
// instead of a state change it has to go looking for. It carries the private
// key, so it is never logged and never leaves this process.
func InstallProfile(doc string) error {
	if module == nil {
		return ErrUnsupported
	}

	profile, err := ParseProfile(doc)
	if err != nil {
		return err
	}

	module.profileLock.Lock()
	module.wanted = profile
	module.profileLock.Unlock()

	log.Infof("wireguard: installed profile %q", profile.Name)
	module.nudge()

	return nil
}

// ClearProfile removes the installed profile and takes the device down.
func ClearProfile() error {
	if module == nil {
		return ErrUnsupported
	}

	module.profileLock.Lock()
	module.wanted = nil
	module.profileLock.Unlock()

	log.Info("wireguard: cleared profile")
	module.nudge()

	return nil
}

// publish stores the snapshot and pushes it to the app.
func (w *WireGuard) publish(s *Snapshot) {
	previous := Status()
	store(s)
	if previous.State != s.State {
		log.Infof("wireguard: %s -> %s", previous.State, s.State)
	}
	w.pushState(s)
}

var (
	module     *WireGuard
	shimLoaded atomic.Bool

	errOnlyOneInstance = errors.New("only one instance allowed")
)

// New returns a new WireGuard module.
func New(instance instance) (*WireGuard, error) {
	if !shimLoaded.CompareAndSwap(false, true) {
		return nil, errOnlyOneInstance
	}
	m := mgr.New("WireGuard")
	module = &WireGuard{
		mgr:      m,
		instance: instance,
		wake:     make(chan struct{}, 1),
	}

	if err := module.prep(); err != nil {
		return nil, err
	}

	return module, nil
}

type instance interface {
	API() *api.API
	Config() *config.Config
	NetEnv() *netenv.NetEnv
	Routing() *routing.Routing
}
