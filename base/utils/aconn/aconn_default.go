//go:build !android

package aconn

import (
	"errors"
	"net"
)

// ErrNotSupported is returned by the desktop stubs, which cannot attribute owners.
var ErrNotSupported = errors.New("connection owner lookup is only available on android")

// RegisterOwnerResolver is a no-op off Android.
func RegisterOwnerResolver(_ OwnerResolver) {}

// HasOwnerResolver reports whether owner attribution is available.
func HasOwnerResolver() bool { return false }

// RegisterAppInfoProvider is a no-op off Android.
func RegisterAppInfoProvider(_ AppInfoProvider) {}

// HasAppInfoProvider reports whether app details are available.
func HasAppInfoProvider() bool { return false }

// LookupUID always reports ErrNotSupported off Android.
func LookupUID(_ uint8, _ net.IP, _ uint16, _ net.IP, _ uint16) (int, error) {
	return UnknownUID, ErrNotSupported
}

// InvalidateAppInfo is a no-op off Android.
func InvalidateAppInfo(_ int) {}

// GetAppInfo always reports ErrNotSupported off Android.
func GetAppInfo(_ int) (*AppInfo, error) {
	return nil, ErrNotSupported
}
