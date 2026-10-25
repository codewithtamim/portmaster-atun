//go:build !android

package anotif

// RegisterListener is a no-op outside Android.
func RegisterListener(_ Listener) {}

// Handles is false outside Android, so callers use the notifications module.
func Handles() bool { return false }

// AlreadyDelivered is always false outside Android.
func AlreadyDelivered(_ string) bool { return false }

// Deliver is a no-op outside Android.
func Deliver(_, _, _ string) bool { return false }

// Retract is a no-op outside Android.
func Retract(_ string) {}
