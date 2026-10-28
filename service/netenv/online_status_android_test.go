//go:build android

package netenv

import (
	"testing"
	"time"
)

func TestStatusCheckInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status OnlineStatus
		want   time.Duration
		ok     bool
	}{
		{StatusOffline, 10 * time.Second, true},
		{StatusPortal, 60 * time.Second, true},
		{StatusLimited, 30 * time.Second, true},
		{StatusSemiOnline, 60 * time.Second, true},
		{StatusOnline, 0, false},
		{StatusUnknown, 5 * time.Minute, true},
	}
	for _, tt := range tests {
		got, ok := statusCheckInterval(tt.status)
		if got != tt.want || ok != tt.ok {
			t.Errorf("statusCheckInterval(%s) = %s, %t; want %s, %t", tt.status, got, ok, tt.want, tt.ok)
		}
	}
}
