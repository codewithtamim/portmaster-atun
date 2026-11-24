//go:build !android

package geoip

import (
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// Available reports whether a location lookup for ip can work.
func Available(net.IP) bool {
	return true
}

func getReader(ip net.IP) *maxminddb.Reader {
	isV6 := ip.To4() == nil
	return worker.GetReader(isV6, true)
}
