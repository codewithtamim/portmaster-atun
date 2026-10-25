//go:build android

package dnsmonitor

// Listener is unused on Android. DNS is intercepted on the TUN.
type Listener struct{}

func newListener(_ *DNSMonitor) (*Listener, error) {
	return &Listener{}, nil
}

func (l *Listener) flush() error {
	return nil
}

func (l *Listener) stop() error {
	return nil
}
