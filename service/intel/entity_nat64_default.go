//go:build !android

package intel

import "net"

// MatchIP is the address rules, filter lists and location data are checked
// against. Outside Android it is the destination address itself.
func (e *Entity) MatchIP() net.IP {
	return e.IP
}
