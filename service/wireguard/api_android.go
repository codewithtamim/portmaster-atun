//go:build android

package wireguard

import (
	"net/http"
	"sync"
	"time"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/runtime"
)

const (
	wireGuardAPIPath = "wireguard"

	// runtimeKey is what the app subscribes to over the database websocket. The
	// app gets pushed state instead of polling the device, which is what the old
	// two-second poll through JNI was doing.
	runtimeKey   = "wireguard/state"
	runtimeDBKey = "runtime:" + runtimeKey

	// Counters move constantly. Publishing every tick would be a websocket
	// message every few seconds for numbers nobody is watching that closely, so
	// only a state change is immediate and the rest is rate limited.
	statePushInterval = time.Second
)

// StateRecord is the runtime database record the app subscribes to.
//
// It carries no key material: a profile is described by its id, name and which
// address families it can carry, and nothing else.
type StateRecord struct {
	record.Base
	sync.Mutex

	State         string `json:"state"`
	Profile       Meta   `json:"profile"`
	Endpoint      string `json:"endpoint"`
	LastHandshake int64  `json:"lastHandshake"`
	RxBytes       uint64 `json:"rxBytes"`
	TxBytes       uint64 `json:"txBytes"`
	ErrCode       string `json:"errCode"`
	ErrDetail     string `json:"errDetail"`
}

func (w *WireGuard) registerAPI() error {
	if err := w.setupRuntimeProvider(); err != nil {
		return err
	}
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "WireGuard state",
		Description: "GET the state of the WireGuard tunnel. Never returns key material.",
		Path:        wireGuardAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		StructFunc:  handleWireGuard,
	})
}

func (w *WireGuard) setupRuntimeProvider() error {
	provider := runtime.SimpleValueGetterFunc(func(_ string) ([]record.Record, error) {
		snapshot := Status()
		return []record.Record{buildStateRecord(&snapshot)}, nil
	})

	push, err := runtime.Register(runtimeKey, provider)
	if err != nil {
		return err
	}
	w.publishUpdate = push

	return nil
}

func handleWireGuard(_ *api.Request) (interface{}, error) {
	snapshot := Status()
	return StateOf(&snapshot), nil
}

// pushState sends the snapshot to subscribers.
//
// A change of state, profile or error goes out at once; a run of counter-only
// updates is collapsed to one message per statePushInterval.
func (w *WireGuard) pushState(s *Snapshot) {
	if w.publishUpdate == nil {
		return
	}

	pushed := pushedState{State: s.State, Profile: s.Profile, ErrCode: s.ErrCode}
	w.pushLock.Lock()
	if pushed == w.lastPushed && time.Since(w.lastPushedAt) < statePushInterval {
		w.pushLock.Unlock()
		return
	}
	w.lastPushed = pushed
	w.lastPushedAt = time.Now()
	w.pushLock.Unlock()

	rec := buildStateRecord(s)
	rec.Lock()
	defer rec.Unlock()

	w.publishUpdate(rec)
}

func buildStateRecord(s *Snapshot) *StateRecord {
	rec := &StateRecord{}
	view := StateOf(s)
	rec.State = view.State
	rec.Profile = view.Profile
	rec.Endpoint = view.Endpoint
	rec.LastHandshake = view.LastHandshake
	rec.RxBytes = view.RxBytes
	rec.TxBytes = view.TxBytes
	rec.ErrCode = view.ErrCode
	rec.ErrDetail = view.ErrDetail

	rec.CreateMeta()
	rec.SetKey(runtimeDBKey)

	return rec
}
