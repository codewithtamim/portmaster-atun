//go:build android

package resolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
)

const dnsSettingsAPIPath = "dns/settings"

// blockSecureDNSKey is profile.CfgOptionPreventBypassingKey. Kept as a
// string so this package does not import profile.
const blockSecureDNSKey = "filter/preventBypassing"

var errUnknownDNSPreset = errors.New("unknown DNS preset")

// DNSPreset is one of the four Replace quick settings the Android radios use.
type DNSPreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Nameservers []string `json:"nameservers"`
	IsDefault   bool     `json:"isDefault"`
}

// DNSSettingsSnapshot is the HTTP payload for the DNS Server screen.
type DNSSettingsSnapshot struct {
	Nameservers    []string    `json:"nameservers"`
	SelectedPreset string      `json:"selectedPreset"`
	UseSpnLookups  bool        `json:"useSpnLookups"`
	BlockSecureDNS bool        `json:"blockSecureDns"`
	Presets        []DNSPreset `json:"presets"`
}

type dnsSettingsBody struct {
	Preset         *string `json:"preset"`
	UseSpnLookups  *bool   `json:"useSpnLookups"`
	BlockSecureDNS *bool   `json:"blockSecureDns"`
}

// dnsPresets adds Android radio ids on top of the nameserver lists shared with
// the desktop QuickReplace settings in config.go. The "Add Cloudflare as
// fallback" merge is not a radio and is omitted.
var dnsPresets = []DNSPreset{
	{
		ID:          "cloudflare",
		Name:        "Cloudflare",
		Nameservers: dnsPresetCloudflare,
		IsDefault:   true,
	},
	{
		ID:          "quad9",
		Name:        "Quad9",
		Nameservers: dnsPresetQuad9,
	},
	{
		ID:          "adguard",
		Name:        "AdGuard",
		Nameservers: dnsPresetAdGuard,
	},
	{
		ID:          "fap",
		Name:        "Foundation for Applied Privacy",
		Nameservers: dnsPresetAppliedPrivacy,
	},
}

func handleDNSSettings(ar *api.Request) (interface{}, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		return putDNSSettings(ar)
	}
	return currentDNSSettings(), nil
}

func putDNSSettings(ar *api.Request) (interface{}, error) {
	var body dnsSettingsBody
	if len(ar.InputData) > 0 {
		if err := json.Unmarshal(ar.InputData, &body); err != nil {
			return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
		}
	}

	if body.Preset != nil {
		preset, ok := presetByID(*body.Preset)
		if !ok {
			return nil, api.ErrorWithStatus(
				fmt.Errorf("%w: %q", errUnknownDNSPreset, *body.Preset),
				http.StatusBadRequest,
			)
		}
		if err := config.SetConfigOption(CfgOptionNameServersKey, preset.Nameservers); err != nil {
			return nil, err
		}
	}

	if body.UseSpnLookups != nil {
		if err := config.SetConfigOption(CfgOptionUseSPNKey, *body.UseSpnLookups); err != nil {
			return nil, err
		}
	}

	if body.BlockSecureDNS != nil {
		if err := config.SetConfigOption(blockSecureDNSKey, *body.BlockSecureDNS); err != nil {
			return nil, err
		}
	}

	return currentDNSSettings(), nil
}

func currentDNSSettings() DNSSettingsSnapshot {
	nameservers := currentNameServers()
	useSPN := true
	if useSpnLookups != nil {
		useSPN = useSpnLookups()
	}

	presets := append([]DNSPreset(nil), dnsPresets...)
	return DNSSettingsSnapshot{
		Nameservers:    nameservers,
		SelectedPreset: matchingPresetID(nameservers),
		UseSpnLookups:  useSPN,
		BlockSecureDNS: blockSecureDNSEnabled(),
		Presets:        presets,
	}
}

func blockSecureDNSEnabled() bool {
	option := config.Concurrent.GetAsBool(blockSecureDNSKey, false)
	if option == nil {
		return false
	}
	return option()
}

func currentNameServers() []string {
	if configuredNameServers == nil {
		return append([]string(nil), defaultNameServers...)
	}
	nameservers := configuredNameServers()
	if nameservers == nil {
		return []string{}
	}
	return nameservers
}

func matchingPresetID(nameservers []string) string {
	for _, preset := range dnsPresets {
		if slices.Equal(preset.Nameservers, nameservers) {
			return preset.ID
		}
	}
	return ""
}

func presetByID(id string) (DNSPreset, bool) {
	for _, preset := range dnsPresets {
		if preset.ID == id {
			return preset, true
		}
	}
	return DNSPreset{}, false
}

func registerDNSSettingsAPI() error {
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "DNS Settings",
		Description: "GET the DNS server presets, SPN lookup flag, and secure-DNS block, PUT a preset, useSpnLookups, and/or blockSecureDns.",
		Path:        dnsSettingsAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleDNSSettings,
	})
}
