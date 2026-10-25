//go:build !android

package resolver

func documentationNamesResolvedGlobally(string) bool {
	return false
}
