//go:build !android

package captain

// uplinkChanged never fires off Android: a nil channel blocks forever, so the
// client loop behaves as upstream.
func uplinkChanged() <-chan struct{} { return nil }

func ackUplinkChanged() {}
