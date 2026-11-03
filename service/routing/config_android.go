//go:build android

package routing

import (
	"github.com/safing/portmaster/base/config"
)

// cfgOptionEnableSPNKey mirrors spn/captain.CfgOptionEnableSPNKey.
// Copied to avoid importing spn from service.
const cfgOptionEnableSPNKey = "spn/enable"

// cfgOptionUseSPNKey mirrors profile.CfgOptionUseSPNKey.
const cfgOptionUseSPNKey = "spn/use"

func registerConfig() error {
	if err := registerTargetConfig(); err != nil {
		return err
	}
	return config.Register(&config.Option{
		Name: "Block When Routing Target Is Down",
		Key:  CfgOptionLockdownKey,
		Description: "While SPN or the WireGuard tunnel is down, block traffic that should go " +
			"through it instead of sending it out directly.",
		OptType:      config.OptTypeBool,
		DefaultValue: true,
		Annotations: config.Annotations{
			config.CategoryAnnotation:     "Routing",
			config.DisplayOrderAnnotation: 2,
		},
	})
}

func registerTargetConfig() error {
	return config.Register(&config.Option{
		Name: "Routing Target",
		Key:  CfgOptionTargetKey,
		Description: "Where accepted connections go after filtering. Filtering applies to all " +
			"of them. Only one target is active at a time.",
		OptType:      config.OptTypeString,
		DefaultValue: string(TargetDirect),
		PossibleValues: []config.PossibleValue{
			{
				Name:        "Direct",
				Value:       string(TargetDirect),
				Description: "Straight to the internet.",
			},
			{
				Name:        "SPN",
				Value:       string(TargetSPN),
				Description: "Through the SPN onion-routing network.",
			},
			{
				Name:        "WireGuard",
				Value:       string(TargetWireGuard),
				Description: "Through a WireGuard tunnel. Traffic is blocked while the tunnel is down.",
			},
		},
		Annotations: config.Annotations{
			config.CategoryAnnotation:     "Routing",
			config.DisplayOrderAnnotation: 1,
		},
	})
}
