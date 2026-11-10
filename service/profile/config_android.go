//go:build android

package profile

import "github.com/safing/portmaster/base/config"

var (
	// CfgOptionBlockOnMobileKey is the config key for blocking apps on mobile data.
	CfgOptionBlockOnMobileKey   = "filter/blockOnMobile"
	cfgOptionBlockOnMobile      config.BoolOption
	cfgOptionBlockOnMobileOrder = 21

	// CfgOptionBlockOnLockKey is the config key for blocking apps while the phone is locked.
	CfgOptionBlockOnLockKey   = "filter/blockOnLock"
	cfgOptionBlockOnLock      config.BoolOption
	cfgOptionBlockOnLockOrder = 22

	// CfgOptionBlockOnWifiKey is the config key for blocking apps on Wi-Fi.
	CfgOptionBlockOnWifiKey   = "filter/blockOnWifi"
	cfgOptionBlockOnWifi      config.BoolOption
	cfgOptionBlockOnWifiOrder = 23
)

func registerAndroidDevicePolicyOptions() error {
	options := []struct {
		name        string
		description string
		key         string
		order       int
		target      *config.BoolOption
	}{
		{
			name:        "Block Apps on Mobile Data",
			description: "Block outgoing connections from apps while the device is on mobile data. Per-app settings can allow or deny an exemption.",
			key:         CfgOptionBlockOnMobileKey,
			order:       cfgOptionBlockOnMobileOrder,
			target:      &cfgOptionBlockOnMobile,
		},
		{
			name:        "Block Apps While Locked",
			description: "Block outgoing connections from apps while the phone is locked. System and notification traffic stays allowed.",
			key:         CfgOptionBlockOnLockKey,
			order:       cfgOptionBlockOnLockOrder,
			target:      &cfgOptionBlockOnLock,
		},
		{
			name:        "Block Apps on Wi-Fi",
			description: "Block outgoing connections from an app while the device is on Wi-Fi.",
			key:         CfgOptionBlockOnWifiKey,
			order:       cfgOptionBlockOnWifiOrder,
			target:      &cfgOptionBlockOnWifi,
		},
	}

	for _, option := range options {
		err := config.Register(&config.Option{
			Name:         option.name,
			Key:          option.key,
			Description:  option.description,
			OptType:      config.OptTypeBool,
			DefaultValue: false,
			Annotations: config.Annotations{
				config.SettablePerAppAnnotation: true,
				config.DisplayOrderAnnotation:   option.order,
				config.CategoryAnnotation:       "Android",
			},
		})
		if err != nil {
			return err
		}
		*option.target = config.Concurrent.GetAsBool(option.key, false)
		cfgBoolOptions[option.key] = *option.target
	}

	return nil
}

// preventBypassingDefault is off until the user turns on "Block apps' own
// secure DNS". Desktop keeps the option on; see config_default.go.
func preventBypassingDefault() bool { return false }

// removeOutOfScopeDNSDefault is off on Android: local-network traffic stays
// outside the tunnel, so public names that point at LAN addresses
// (plex.direct, nip.io, router and NAS apps) must resolve. The app states
// that it lacks desktop's DNS-rebinding protection rather than enforcing it.
// An explicit user value still wins.
func removeOutOfScopeDNSDefault() bool { return false }

func applyAndroidDevicePolicyOptions(lp *LayeredProfile) {
	lp.BlockOnMobile = lp.wrapBoolOption(
		CfgOptionBlockOnMobileKey,
		cfgOptionBlockOnMobile,
	)
	lp.BlockOnLock = lp.wrapBoolOption(
		CfgOptionBlockOnLockKey,
		cfgOptionBlockOnLock,
	)
	lp.BlockOnWifi = lp.wrapBoolOption(
		CfgOptionBlockOnWifiKey,
		cfgOptionBlockOnWifi,
	)
}
