//go:build android

package captain

import "time"

func init() {
	// Screen-on health checks are the radio keepalive. Three minutes stays
	// under typical mobile TCP NAT timeouts. Screen-off keeps the default.
	clientHealthCheckTickDuration = 3 * time.Minute
}
