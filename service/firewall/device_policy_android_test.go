//go:build android

package firewall

import (
	"context"
	"testing"

	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/profile"
)

func TestSystemUIDStillBlockedOnWifiAndMobile(t *testing.T) { //nolint:paralleltest // device state is global
	t.Cleanup(func() {
		_ = aconn.UpdateDeviceStateJSON("")
	})

	wifi := policyProfile(true, false, false)
	if err := aconn.UpdateDeviceStateJSON(`{"wifi":true}`); err != nil {
		t.Fatal(err)
	}
	shell := network.NewPolicyConnection(2000)
	if !checkDevicePolicy(context.Background(), shell, wifi, nil) {
		t.Fatal("uid 2000 on Wi-Fi was not blocked")
	}
	if shell.Reason.Msg != "blocked on Wi-Fi" {
		t.Fatalf("wifi reason %q", shell.Reason.Msg)
	}

	mobile := policyProfile(false, true, false)
	if err := aconn.UpdateDeviceStateJSON(`{"cellular":true}`); err != nil {
		t.Fatal(err)
	}
	shell = network.NewPolicyConnection(2000)
	if !checkDevicePolicy(context.Background(), shell, mobile, nil) {
		t.Fatal("uid 2000 on mobile was not blocked")
	}
	if shell.Reason.Msg != "blocked on mobile data" {
		t.Fatalf("mobile reason %q", shell.Reason.Msg)
	}

	locked := policyProfile(false, false, true)
	if err := aconn.UpdateDeviceStateJSON(`{"locked":true}`); err != nil {
		t.Fatal(err)
	}
	shell = network.NewPolicyConnection(2000)
	if checkDevicePolicy(context.Background(), shell, locked, nil) {
		t.Fatal("uid 2000 was blocked while locked")
	}
	app := network.NewPolicyConnection(androidFirstAppUID)
	if !checkDevicePolicy(context.Background(), app, locked, nil) {
		t.Fatal("app uid was not blocked while locked")
	}
}

func policyProfile(wifi, mobile, lock bool) *profile.LayeredProfile {
	return &profile.LayeredProfile{
		BlockOnWifi:   func() bool { return wifi },
		BlockOnMobile: func() bool { return mobile },
		BlockOnLock:   func() bool { return lock },
	}
}

// On metered LTE the cellular flag may not be reported while the VPN is
// the active network; the metered flag still makes it mobile.
func TestMeteredIsMobile(t *testing.T) { //nolint:paralleltest // device state is global
	t.Cleanup(func() {
		_ = aconn.UpdateDeviceStateJSON("")
	})

	for _, tc := range []struct {
		name               string
		state              string
		blockWifi, blockMo bool
		wantBlocked        bool
		wantReason         string
	}{
		{"metered without transport", `{"metered":true}`, false, true, true, "blocked on mobile data"},
		{"metered wifi is mobile", `{"wifi":true,"metered":true}`, false, true, true, "blocked on mobile data"},
		{"metered wifi is not wifi", `{"wifi":true,"metered":true}`, true, false, false, ""},
		{"unmetered wifi", `{"wifi":true}`, false, true, false, ""},
		{"unmetered cellular is mobile", `{"cellular":true}`, false, true, true, "blocked on mobile data"},
	} {
		if err := aconn.UpdateDeviceStateJSON(tc.state); err != nil {
			t.Fatal(err)
		}
		conn := network.NewPolicyConnection(androidFirstAppUID)
		got := checkDevicePolicy(context.Background(), conn, policyProfile(tc.blockWifi, tc.blockMo, false), nil)
		if got != tc.wantBlocked {
			t.Errorf("%s: blocked = %v, want %v", tc.name, got, tc.wantBlocked)
		}
		if got && conn.Reason.Msg != tc.wantReason {
			t.Errorf("%s: reason %q, want %q", tc.name, conn.Reason.Msg, tc.wantReason)
		}
	}
}
