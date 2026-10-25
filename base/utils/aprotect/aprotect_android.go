//go:build android

// Package aprotect provides a helper for protecting sockets.
//
// On Android, all real outbound sockets must be excluded from the TUN
// via VpnService.protect(fd), or their replies loop back into the TUN.
// https://developer.android.com/reference/android/net/VpnService#protect(int)
package aprotect

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/safing/portmaster/base/utils"
)

var (
	callbackMu sync.RWMutex
	protector  Protector

	errProtectFailed = errors.New("aprotect: failed to protect socket")
)

func init() {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		http.DefaultTransport = ApplyToTransport(transport.Clone())
	}
}

// RegisterProtector must be called from a class that extends VpnService.
func RegisterProtector(p Protector) {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	if utils.IsNilCallback(p) {
		protector = nil
		return
	}
	protector = p
}

func HasProtector() bool {
	callbackMu.RLock()
	defer callbackMu.RUnlock()
	return !utils.IsNilCallback(protector)
}

func ProtectFD(fd int) bool {
	callbackMu.RLock()
	p := protector
	callbackMu.RUnlock()

	if utils.IsNilCallback(p) {
		// TUN is not up yet. protect() is only required once VpnService
		// owns the default route, otherwise intel/DNS dials fail at boot.
		return true
	}

	return p.Protect(int32(fd))
}

func protectRawConn(c syscall.RawConn) error {
	var protectErr error
	if err := c.Control(func(fd uintptr) {
		if !ProtectFD(int(fd)) {
			protectErr = errProtectFailed
		}
	}); err != nil {
		return err
	}
	return protectErr
}

func ApplyToDialer(dialer *net.Dialer) *net.Dialer {
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	prevControl := dialer.Control
	prevControlContext := dialer.ControlContext
	dialer.Control = nil
	dialer.ControlContext = func(ctx context.Context, network, address string, c syscall.RawConn) error {
		if err := protectRawConn(c); err != nil {
			return err
		}

		switch {
		case prevControlContext != nil:
			return prevControlContext(ctx, network, address, c)
		case prevControl != nil:
			return prevControl(network, address, c)
		default:
			return nil
		}
	}
	return dialer
}

func NewDialer() *net.Dialer {
	return ApplyToDialer(&net.Dialer{})
}

func ApplyToTransport(transport *http.Transport) *http.Transport {
	if transport == nil {
		if def, ok := http.DefaultTransport.(*http.Transport); ok {
			return ApplyToTransport(def.Clone())
		}
		transport = &http.Transport{}
	}
	// Same timeouts as the dialer of http.DefaultTransport, which this
	// replaces in init.
	transport.DialContext = ApplyToDialer(&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return transport
}
