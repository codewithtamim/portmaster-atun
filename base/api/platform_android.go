//go:build android

package api

import (
	"crypto/subtle"
	"net"
	"sync/atomic"

	"github.com/safing/portmaster/base/log"
)

// devModeGrantsAccess is false on Android. On desktop, Development Mode lets
// any local client in, because only the user's own processes share the
// machine. On Android every installed app shares 127.0.0.1, so the bypass
// would hand any app full control of the firewall. Clients must always
// present the app's API key.
func devModeGrantsAccess() bool {
	return false
}

// platformListenAddress keeps the API on loopback. The address is a setting,
// and a non-loopback one would expose the API to the whole Wi-Fi network.
func platformListenAddress(configured string) string {
	if isLoopbackAddress(configured) {
		return configured
	}
	fallback := defaultListenAddress
	if !isLoopbackAddress(fallback) {
		// An empty default would listen on every interface.
		fallback = "127.0.0.1:3008"
	}

	log.Warningf("api: ignoring non-loopback listen address %q, using %s", configured, fallback)
	return fallback
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// processAPIKey is the app UI's key. It exists only in memory, for the life
// of the process: it is never written to config, so it cannot leak through
// a settings export or outlive the app process that knows it.
var processAPIKey atomic.Pointer[string]

// processAPIToken is what the process key grants: the app UI is the admin.
var processAPIToken = AuthToken{Read: PermitAdmin, Write: PermitAdmin}

// SetProcessAPIKey sets the in-memory API key the app UI presents as
// "Authorization: Bearer <key>". An empty key disables it.
func SetProcessAPIKey(key string) {
	if key == "" {
		processAPIKey.Store(nil)
		return
	}
	processAPIKey.Store(&key)
}

// platformAPIKey matches key against the process key in constant time.
func platformAPIKey(key string) (*AuthToken, bool) {
	want := processAPIKey.Load()
	if want == nil || subtle.ConstantTimeCompare([]byte(key), []byte(*want)) != 1 {
		return nil, false
	}
	// A copy: checkAPIKey's caller must not be able to change the template.
	token := processAPIToken
	return &token, true
}
