//go:build android

package nameserver

func applyPlatformNameserverDefault() {
	// App processes cannot bind ports below 1024. The TUN integration sends
	// the queries it is told to reroute here.
	defaultNameserverAddress = "127.0.0.1:0"
}
