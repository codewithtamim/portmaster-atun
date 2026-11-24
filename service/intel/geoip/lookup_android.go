//go:build android

package geoip

import (
	"net"
	"os"

	"github.com/oschwald/maxminddb-golang"
)

// getReader waits for a database only while its file is installed. The app
// downloads GeoIP once SPN, an account or a country rule needs it. Until then
// a wait could only time out, and it holds the first packet of every new
// connection for a second. The databases load once the updater announces the
// downloaded files (see Start).
func getReader(ip net.IP) *maxminddb.Reader {
	isV6 := ip.To4() == nil
	if reader := worker.GetReader(isV6, false); reader != nil {
		return reader
	}
	return worker.GetReader(isV6, databaseInstalled(isV6))
}

// Available reports whether a location lookup for ip can work: false until
// the app has downloaded GeoIP.
func Available(ip net.IP) bool {
	isV6 := ip.To4() == nil
	return worker.GetReader(isV6, false) != nil || databaseInstalled(isV6)
}

func databaseInstalled(v6 bool) bool {
	name := v4MMDBResource
	if v6 {
		name = v6MMDBResource
	}
	artifact, err := module.instance.IntelUpdates().GetFile(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(artifact.Path())
	return err == nil
}
