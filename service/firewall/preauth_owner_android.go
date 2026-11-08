//go:build android

package firewall

import (
	"os"

	"github.com/safing/portmaster/service/network"
)

var ownUID = os.Getuid()

// preAuthOwnerMatches reports whether conn may use a pre-authenticated port.
// A port number alone is not enough here: another app can hold the same port
// on another address while our entry is live. Only our own UID may use it.
func preAuthOwnerMatches(conn *network.Connection) bool {
	return conn.UID != nil && *conn.UID == ownUID
}
