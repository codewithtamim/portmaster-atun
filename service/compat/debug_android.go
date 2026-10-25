//go:build android

package compat

import "github.com/safing/portmaster/base/utils/debug"

// AddToDebugInfo is a no-op on Android: interception runs through the TUN, so
// there are no firewall rules to report.
func AddToDebugInfo(_ *debug.Info) {}
