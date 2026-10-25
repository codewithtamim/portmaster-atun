//go:build android

package resolver

import (
	"context"
	"sync/atomic"

	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
)

// CfgOptionUseSPNKey controls whether resolver sockets enter TUN/SPN.
// Default true matches the Android DNS screen. Lookups still go direct
// until SPN is connected, see resolverShouldUseSPN.
const CfgOptionUseSPNKey = "dns/useSPN"

const cfgOptionUseSPNOrder = 6

func registerAndroidDNSConfig() error {
	err := config.Register(&config.Option{
		Name:           "Send DNS Lookups Through SPN",
		Key:            CfgOptionUseSPNKey,
		Description:    "When your route is SPN, send DNS lookups through it so the DNS server sees an SPN address instead of yours. Turn off to send lookups directly. Slightly faster, less private.",
		OptType:        config.OptTypeBool,
		ExpertiseLevel: config.ExpertiseLevelUser,
		ReleaseLevel:   config.ReleaseLevelStable,
		DefaultValue:   true,
		Annotations: config.Annotations{
			config.DisplayOrderAnnotation: cfgOptionUseSPNOrder,
			config.CategoryAnnotation:     "Servers",
		},
	})
	if err != nil {
		return err
	}
	useSpnLookups = config.Concurrent.GetAsBool(CfgOptionUseSPNKey, true)
	return nil
}

// prevUseSPN is atomic because config change callbacks run concurrently.
var prevUseSPN atomic.Bool

func captureUseSPN() {
	if useSpnLookups != nil {
		prevUseSPN.Store(useSpnLookups())
	}
}

func onUseSPNConfigChange(ctx context.Context) {
	if useSpnLookups == nil {
		return
	}
	newUseSPN := useSpnLookups()
	if prevUseSPN.Swap(newUseSPN) == newUseSPN {
		return
	}
	ForceResolverReconnect(ctx)
	log.Debug("resolver: forced reconnect after dns/useSPN change")
}
