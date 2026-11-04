//go:build !android

package service

import "github.com/safing/portmaster/service/mgr"

// interceptionModuleForServiceGroup includes TUN interception in the main
// start path. Desktop kernels need capture as soon as the core is up.
func interceptionModuleForServiceGroup(m *mgr.GroupModule) mgr.Module {
	return m
}
