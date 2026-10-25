package firewall

import (
	"net"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/network/netutils"
)

var (
	apiPortSet bool
	apiIP      net.IP
	apiPort    uint16
)

func prepAPIAuth() error {
	return api.SetAuthenticator(apiAuthenticator)
}

func startAPIAuth() {
	var err error
	apiIP, apiPort, err = netutils.ParseIPPort(apiListenAddress())
	if err != nil {
		log.Warningf("filter: failed to parse API address for improved api auth mechanism: %s", err)
		return
	}
	apiPortSet = true
	log.Tracef("filter: api port set to %d", apiPort)
}
