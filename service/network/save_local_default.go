//go:build !android

package network

func skipLocalNetworkSave(_ *Connection) bool {
	return false
}
