//go:build !android

package aconn

import (
	"errors"
	"net"
)

var ErrNotSupported = errors.New("connection owner lookup is only available on android")

func RegisterOwnerResolver(_ OwnerResolver) {}

func HasOwnerResolver() bool { return false }

func RegisterAppInfoProvider(_ AppInfoProvider) {}

func HasAppInfoProvider() bool { return false }

func LookupUID(_ uint8, _ net.IP, _ uint16, _ net.IP, _ uint16) (int, error) {
	return UnknownUID, ErrNotSupported
}

func InvalidateAppInfo(_ int) {}

func GetAppInfo(_ int) (*AppInfo, error) {
	return nil, ErrNotSupported
}
