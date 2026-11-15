//go:build android

package status

import (
	"slices"
	"testing"

	"github.com/safing/portmaster/service/mgr"
)

// Only errors the app has no other place for are reported, in a stable order.
func TestHealthProblems(t *testing.T) {
	t.Parallel()

	updates := []mgr.StateUpdate{
		{Module: "Resolver", States: []mgr.State{
			{ID: "resolver:all-configured-resolvers-failed", Type: mgr.StateTypeError},
		}},
		{Module: "Nameserver", States: []mgr.State{
			{ID: "nameserver:listener-failed", Type: mgr.StateTypeError},
			{ID: "nameserver:slow", Type: mgr.StateTypeWarning},
		}},
		{Module: "Captain", States: []mgr.State{
			{ID: "spn:not-logged-in", Type: mgr.StateTypeError},
		}},
		{Module: "Config", States: []mgr.State{
			{ID: "config:validation-errors-on-load", Type: mgr.StateTypeError},
		}},
	}

	want := []HealthProblem{
		{Module: "Config", ID: "config:validation-errors-on-load"},
		{Module: "Nameserver", ID: "nameserver:listener-failed"},
	}
	if got := healthProblems(updates); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := healthProblems(nil); got == nil || len(got) != 0 {
		t.Fatalf("no states must give an empty list, got %v", got)
	}
}
