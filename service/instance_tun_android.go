//go:build android

package service

import (
	"sync"

	"github.com/safing/portmaster/service/firewall/interception/atun"
)

// tunLock serializes StartTun and StopTun. Both are called from Java, and
// their check, stop and start steps must not interleave.
var tunLock sync.Mutex

// StartTun attaches the TUN VpnService established and starts interception.
// The fd does not exist at boot, so the interception group is started here.
// Invalid parameters are rejected before anything running is touched.
func (i *Instance) StartTun(fd, mtu int, tunAddr, fakeDNS string) error {
	tunLock.Lock()
	defer tunLock.Unlock()

	cfg, err := atun.NewTunConfig(fd, mtu, tunAddr, fakeDNS)
	if err != nil {
		return err
	}

	// Group Start is a no-op while running, so a new fd would be ignored.
	// Leave a running stack alone when nothing changed; otherwise restart it.
	if atun.Running() {
		if current, ok := atun.CurrentTunConfig(); ok && current == cfg {
			return nil
		}
		if err := i.serviceGroupInterception.Stop(); err != nil {
			return err
		}
	}

	atun.SetTunConfig(cfg)
	return i.serviceGroupInterception.Start()
}

// StopTun stops interception and forgets the TUN.
func (i *Instance) StopTun() error {
	tunLock.Lock()
	defer tunLock.Unlock()

	err := i.serviceGroupInterception.Stop()
	atun.ClearTunConfig()
	return err
}
