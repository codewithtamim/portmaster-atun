//go:build android

package service

import "github.com/safing/portmaster/service/mgr"

// interceptionModuleForServiceGroup keeps interception out of the boot group.
// Android has no TUN fd until VpnService hands one over through StartTun;
// starting the group early would fail Open with ErrNoTunConfig.
//
// Return a *deferredModule, not nil and not a struct value: mgr.Group.Add calls
// Manager() before its nil checks, so a nil module panics, and its
// reflect.Value.IsNil check panics on a non-pointer. A nil manager is skipped.
func interceptionModuleForServiceGroup(_ *mgr.GroupModule) mgr.Module {
	return &deferredModule{}
}

// deferredModule stands in for a module that is started outside the boot
// group. Its nil manager makes mgr.Group.Add skip it.
type deferredModule struct{}

func (*deferredModule) Manager() *mgr.Manager { return nil }
func (*deferredModule) Start() error          { return nil }
func (*deferredModule) Stop() error           { return nil }
