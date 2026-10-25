//go:build !android

package resolver

import "context"

func registerAndroidDNSConfig() error { return nil }

func captureUseSPN() {}

func onUseSPNConfigChange(context.Context) {}
