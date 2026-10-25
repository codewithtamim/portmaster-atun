//go:build !android

package aprotect

import (
	"net"
	"net/http"
)

func RegisterProtector(_ Protector) {}

func HasProtector() bool { return false }

func ProtectFD(_ int) bool { return true }

func ApplyToDialer(dialer *net.Dialer) *net.Dialer {
	if dialer == nil {
		return &net.Dialer{}
	}
	return dialer
}

func NewDialer() *net.Dialer {
	return &net.Dialer{}
}

func ApplyToTransport(transport *http.Transport) *http.Transport {
	if transport == nil {
		return &http.Transport{}
	}
	return transport
}
