//go:build android

package interop

import "github.com/safing/portmaster/service/mgr"

// Interoperability is a no-op on Android. Desktop IVPN/API interop is
// excluded with !android, but service.New still constructs this module.
type Interoperability struct {
	mgr *mgr.Manager
}

func New(_ any) (*Interoperability, error) {
	return &Interoperability{mgr: mgr.New("Interop")}, nil
}

func (u *Interoperability) Manager() *mgr.Manager {
	return u.mgr
}

func (u *Interoperability) Start() error {
	return nil
}

func (u *Interoperability) Stop() error {
	return nil
}
