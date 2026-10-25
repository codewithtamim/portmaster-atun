//go:build !android

package endpoints

// deniedExactMatchesSubdomain keeps desktop matching exact.
// A bare domain blocks only that name, on Linux and Windows.
func deniedExactMatchesSubdomain(_ *EndpointDomain, _ string) bool {
	return false
}
