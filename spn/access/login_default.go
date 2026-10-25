//go:build !android

package access

import "net/http"

// platformLoginCredentials offers no fallback on desktop: SPN login stays
// HTTP Basic only.
func platformLoginCredentials(*http.Request) (username, password string) {
	return "", ""
}
