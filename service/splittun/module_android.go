//go:build android

package splittun

import "github.com/safing/portmaster/service/mgr"

// SplitTunModule is a no-op on Android. Desktop split-tunnel proxies are
// excluded with !android, but service.New still constructs this module.
type SplitTunModule struct {
	mgr *mgr.Manager
}

func New(_ any) (*SplitTunModule, error) {
	return &SplitTunModule{mgr: mgr.New("SplitTunModule")}, nil
}

func (s *SplitTunModule) Manager() *mgr.Manager {
	return s.mgr
}

func (s *SplitTunModule) Start() error {
	return nil
}

func (s *SplitTunModule) Stop() error {
	return nil
}

func IsReady() bool {
	return false
}
