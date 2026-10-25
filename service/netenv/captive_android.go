//go:build android

package netenv

import (
	"net/url"
	"strings"

	"github.com/safing/portmaster/base/api"
)

// ApplyAndroidCaptive records the captive portal Android observed on the
// active network. Setting it moves online status to portal so the firewall
// grants connectivity domains. Clearing it drops that portal.
func ApplyAndroidCaptive(active bool, portalURL string) {
	if !active {
		if GetOnlineStatus() == StatusPortal {
			updateOnlineStatus(StatusUnknown, nil, "android captive portal cleared")
			TriggerOnlineStatusInvestigation()
			return
		}
		setCaptivePortal(nil)
		return
	}
	next := androidPortalURL(portalURL)
	current := GetCaptivePortal()
	if GetOnlineStatus() == StatusPortal && current != nil && current.URL == next.String() {
		return
	}
	// setCaptivePortal keeps the first URL. Clear it so a new Android URL replaces it.
	setCaptivePortal(nil)
	updateOnlineStatus(StatusPortal, next, "android reported captive portal")
}

func androidPortalURL(raw string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
			return parsed
		}
	}
	return parsedPortalTestURL
}

// CaptivePortalAPI is the body of GET network/captive.
type CaptivePortalAPI struct {
	Active bool   `json:"active"`
	URL    string `json:"url"`
}

// CaptivePortalStatus reports whether Android left the device on a portal,
// and the sign-in URL when it is a real page.
func CaptivePortalStatus() CaptivePortalAPI {
	portal := GetCaptivePortal()
	raw := ""
	if portal != nil {
		raw = portal.URL
	}
	if raw == PortalTestURL {
		raw = ""
	}
	return CaptivePortalAPI{
		Active: GetOnlineStatus() == StatusPortal,
		URL:    raw,
	}
}

func registerCaptivePortalAPI() error {
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Get Captive Portal",
		Description: "Returns whether the device is on a captive portal and the sign-in URL.",
		Path:        "network/captive",
		Read:        api.PermitUser,
		StructFunc: func(_ *api.Request) (i interface{}, err error) {
			return CaptivePortalStatus(), nil
		},
	})
}
