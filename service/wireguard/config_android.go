//go:build android

package wireguard

import (
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
)

// CfgOptionConfigSelectedKey records that the app has a WireGuard config
// selected. The profile itself lives only in memory, so after an engine
// restart this is what tells the firewall a tunnel is wanted before the app
// has installed the profile again. Written by InstallProfile and ClearProfile.
const CfgOptionConfigSelectedKey = "wireguard/configSelected"

// configSelected reads CfgOptionConfigSelectedKey. Nil until registerConfig.
var configSelected config.BoolOption

func registerConfig() error {
	err := config.Register(&config.Option{
		Name:           "WireGuard Config Selected",
		Key:            CfgOptionConfigSelectedKey,
		Description:    "Set by the app while a WireGuard config is selected. Not a user setting.",
		OptType:        config.OptTypeBool,
		ExpertiseLevel: config.ExpertiseLevelDeveloper,
		DefaultValue:   false,
	})
	if err != nil {
		return err
	}
	configSelected = config.Concurrent.GetAsBool(CfgOptionConfigSelectedKey, false)
	return nil
}

// ConfigSelected reports whether the app has a WireGuard config selected,
// whether or not its profile is installed yet.
func ConfigSelected() bool {
	return configSelected != nil && configSelected()
}

// WantsTunnel reports whether the WireGuard target has a config to carry
// traffic. False only when the user has no config selected at all; then the
// target cannot do anything and traffic goes direct.
func WantsTunnel() bool {
	return wantsTunnel(Status().Profile.Name, ConfigSelected())
}

func wantsTunnel(profileName string, selected bool) bool {
	return profileName != "" || selected
}

// setConfigSelected persists the selection. It writes only on a change: every
// write is a config change event and a file save.
func setConfigSelected(selected bool) {
	if configSelected == nil || configSelected() == selected {
		return
	}
	var value any = true
	if !selected {
		// Back to the default.
		value = nil
	}
	if err := config.SetConfigOption(CfgOptionConfigSelectedKey, value); err != nil {
		log.Warningf("wireguard: failed to save config selection: %s", err)
	}
}
