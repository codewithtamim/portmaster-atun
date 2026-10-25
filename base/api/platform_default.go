//go:build !android

package api

// devModeGrantsAccess lets Development Mode bypass authentication.
func devModeGrantsAccess() bool {
	return true
}

// platformListenAddress uses the configured address unchanged.
func platformListenAddress(configured string) string {
	return configured
}

// platformAPIKey matches nothing: desktop keys all come from the api/keys setting.
func platformAPIKey(string) (*AuthToken, bool) {
	return nil, false
}
