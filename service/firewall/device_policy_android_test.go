//go:build android

package firewall

import (
	"context"
	"testing"

	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/process"
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

type fakePushAppInfo map[int32]string

func (f fakePushAppInfo) GetAppInfoJSON(uid int32) string { return f[uid] }

func TestPushServicesExemptWhileLocked(t *testing.T) { //nolint:paralleltest // app info provider is global
	aconn.RegisterAppInfoProvider(fakePushAppInfo{
		10150: `{"packageName":"com.google.android.gms","systemApp":true}`,
		10151: `{"packageName":"com.google.android.gms","systemApp":false,"signer":"5f2391277b1dbd489000467e4c2fa6af802430080457dce2f618992e9dfb5402"}`,
		10152: `{"packageName":"com.google.android.gms","systemApp":false,"signer":"00aa"}`,
		10153: `{"packageName":"com.google.android.gsf","systemApp":false}`,
		10154: `{"packageName":"com.google.android.gsf","signer":"11bb","signerHistory":["7ce83c1b71f3d572fed04c8d40c5cb10ff75e6d87d9df6fbd53f0468c2905053","11bb"]}`,
		10200: `{"packageName":"com.example.chat","systemApp":true}`,
	})
	t.Cleanup(func() { aconn.RegisterAppInfoProvider(nil) })

	cases := []struct {
		name string
		proc *process.Process
		want bool
	}{
		{"system", &process.Process{UserID: 1000}, true},
		{"play services", &process.Process{UserID: 10150, ExecName: "com.google.android.gms"}, true},
		{"shared google uid", &process.Process{UserID: 10150, ExecName: "com.google.android.gsf.login", Path: "com.google.uid.shared"}, true},
		{"sandboxed play", &process.Process{UserID: 10151, ExecName: "com.google.android.gms"}, true},
		{"rotated google key", &process.Process{UserID: 10154, ExecName: "com.google.android.gsf"}, true},
		{"sideloaded under gms name", &process.Process{UserID: 10152, ExecName: "com.google.android.gms"}, false},
		{"sideloaded under gsf name, no signer", &process.Process{UserID: 10153, ExecName: "com.google.android.gsf"}, false},
		{"lookup fails", &process.Process{UserID: 10199, ExecName: "com.google.android.gms"}, false},
		{"ordinary app", &process.Process{UserID: 10200, ExecName: "com.example.chat", Path: "/data/app/x/base.apk"}, false},
	}
	for _, c := range cases {
		if got := exemptWhileLocked(c.proc); got != c.want {
			t.Errorf("%s: exemptWhileLocked = %v, want %v", c.name, got, c.want)
		}
	}
}
