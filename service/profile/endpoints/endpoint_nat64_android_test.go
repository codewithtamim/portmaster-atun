//go:build android

package endpoints

import (
	"context"
	"net"
	"testing"

	"github.com/safing/portmaster/service/intel"
)

func TestIPRuleAlsoMatchesTheNat64FormOfTheAddress(t *testing.T) {
	list, err := ParseEndpoints([]string{"- 1.1.1.1", "- 8.8.8.0/24", "+ *"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ip   string
		want EPResult
	}{
		{"1.1.1.1", Denied},
		{"64:ff9b::101:101", Denied},
		{"64:ff9b::808:808", Denied},
		{"64:ff9b::1.0.0.1", Permitted},
		{"2606:4700:4700::1111", Permitted},
	}
	for _, tc := range tests {
		entity := (&intel.Entity{IP: net.ParseIP(tc.ip), Protocol: 6, Port: 443}).Init(443)
		got, _ := list.Match(context.TODO(), entity)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.ip, got, tc.want)
		}
	}
}
