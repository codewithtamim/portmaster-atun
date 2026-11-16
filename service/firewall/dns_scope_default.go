//go:build !android

package firewall

func sharesIPInfoGlobally(_ string) bool {
	return false
}
