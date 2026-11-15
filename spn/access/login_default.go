//go:build !android

package access

import "net/http"

// platformLoginCredentials offers no fallback on desktop: SPN login stays
// HTTP Basic only.
func platformLoginCredentials(*http.Request) (username, password string) {
	return "", ""
}

// logoutDisablesSPN reports whether signing out turns the SPN off, which it
// does on desktop.
func logoutDisablesSPN() bool {
	return true
}

// profileRefreshError returns the error as it is on desktop.
func profileRefreshError(err error, _ int) error {
	return err
}
