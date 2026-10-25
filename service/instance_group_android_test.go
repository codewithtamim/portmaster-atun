//go:build android

package service

import (
	"testing"

	"github.com/safing/portmaster/service/mgr"
)

// TestInterceptionGroupPlaceholderIsSkipped guards against a startup panic:
// mgr.Group.Add calls Manager() and reflect.Value.IsNil on every module, so
// the placeholder Android puts in the boot group must survive both.
func TestInterceptionGroupPlaceholderIsSkipped(t *testing.T) {
	var interception *mgr.GroupModule
	g := mgr.NewGroup(interceptionModuleForServiceGroup(interception))
	if err := g.Start(); err != nil {
		t.Fatalf("empty group failed to start: %s", err)
	}
}
