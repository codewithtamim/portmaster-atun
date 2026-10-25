//go:build android

package firewall

import (
	"net/http"

	"github.com/safing/portmaster/base/api"
)

// apiAuthenticator grants nothing on Android.
//
// Desktop identifies the requesting process through the connection tables and
// trusts Portmaster's own binaries. Android cannot: /proc/net is filtered to
// the calling UID since Android 10, getConnectionOwnerUid only answers for
// flows routed through our own VPN, and every app shares 127.0.0.1. The app
// UI authenticates with the in-memory process key instead (see
// api.SetProcessAPIKey), and everything else is denied.
func apiAuthenticator(_ *http.Request, _ *http.Server) (*api.AuthToken, error) {
	return nil, nil
}
