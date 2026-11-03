//go:build !android

package access

// spnWanted keeps the upstream behavior: the SPN runs whenever it is enabled.
func spnWanted(enabled bool) bool { return enabled }
