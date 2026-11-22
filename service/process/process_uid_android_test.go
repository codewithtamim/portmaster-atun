//go:build android

package process

import (
	"context"
	"testing"

	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/profile"
)

func TestAndroidIdentityUnchanged(t *testing.T) {
	clone := &aconn.AppInfo{PackageName: "com.google.android.gms", Label: "Play", SharedUserID: "com.google.uid.shared", AndroidUser: 10, Signer: "aa"}
	plainClone := &aconn.AppInfo{PackageName: "org.chat", Label: "Chat", SourceDir: "/data/app/x/base.apk", AndroidUser: 10, Signer: "aa"}
	owner := &aconn.AppInfo{PackageName: "com.google.android.gms", Label: "Play", SharedUserID: "com.google.uid.shared", Signer: "aa"}

	cases := []struct {
		name                       string
		pName, path, shared, signr string
		info                       *aconn.AppInfo
		want                       bool
	}{
		{"shared clone keeps its user suffix", "Play", "com.google.uid.shared", "com.google.uid.shared@10", "aa", clone, true},
		{"non-shared clone has no shared tag", "Chat", "/data/app/x/base.apk", "", "aa", plainClone, true},
		{"user 0 shared", "Play", "com.google.uid.shared", "com.google.uid.shared", "aa", owner, true},
		{"label changed", "Old", "com.google.uid.shared", "com.google.uid.shared", "aa", owner, false},
		{"shared tag of another user", "Play", "com.google.uid.shared", "com.google.uid.shared", "aa", clone, false},
		{"signer changed", "Play", "com.google.uid.shared", "com.google.uid.shared", "bb", owner, false},
	}
	for _, c := range cases {
		if got := androidIdentityUnchanged(c.pName, c.path, c.shared, c.signr, c.info); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRefreshDropsAppThatCanNoLongerBeLookedUp(t *testing.T) { //nolint:paralleltest // process storage is global
	aconn.RegisterAppInfoProvider(nil)

	// Seed the unidentified process so the test needs no profile database.
	processesLock.Lock()
	processes[unidentifiedProcess.processKey] = unidentifiedProcess
	processesLock.Unlock()

	key := getAndroidProcessKey(10777)
	stale := &Process{UserID: 10777, Name: "Old app", processKey: key}
	stale.Save()
	t.Cleanup(func() {
		processesLock.Lock()
		delete(processes, key)
		delete(processes, unidentifiedProcess.processKey)
		processesLock.Unlock()
	})

	got := refreshAndroidIdentity(context.Background(), stale)
	if got != unidentifiedProcess {
		t.Fatalf("got %+v, want the unidentified process", got)
	}
	if _, ok := GetProcessFromStorage(key); ok {
		t.Fatal("stale app identity is still stored")
	}
}

// Android's DNS resolver has no app package. Its traffic is the operating
// system's, not an unknown app's.
func TestAndroidDNSResolverIsTheOperatingSystem(t *testing.T) {
	if !IsAndroidSystemUID(androidDNSUID) {
		t.Fatal("DNS resolver UID not part of Android")
	}
	p := &Process{UserID: androidDNSUID}
	if got := p.getSpecialProfileID(); got != profile.SystemProfileID {
		t.Fatalf("profile %q, want %q", got, profile.SystemProfileID)
	}
	if !IsAndroidSystemUID(androidSystemUID) {
		t.Fatal("system server UID not part of Android")
	}
	if IsAndroidSystemUID(10123) {
		t.Fatal("app UID counted as part of Android")
	}
	if IsAndroidSystemUID(rootUID) {
		t.Fatal("root counted as an Android service")
	}
}

func TestStoredAndroidSystemProcessIsKeptWithoutAppInfo(t *testing.T) { //nolint:paralleltest // process storage is global
	aconn.RegisterAppInfoProvider(nil)

	if got := androidSystemAppInfo(androidDNSUID); androidIdentityPath(got) != "android.uid.system" {
		t.Fatalf("path %q, want the system server's identity", androidIdentityPath(got))
	}

	key := getAndroidProcessKey(androidDNSUID)
	stored := newAndroidProcess(androidDNSUID, key, androidSystemAppInfo(androidDNSUID), 1)
	stored.Save()
	t.Cleanup(func() {
		processesLock.Lock()
		delete(processes, key)
		processesLock.Unlock()
	})

	if got := refreshAndroidIdentity(context.Background(), stored); got != stored {
		t.Fatalf("got %+v, want the stored process kept", got)
	}
	if _, ok := GetProcessFromStorage(key); !ok {
		t.Fatal("Android system process dropped")
	}
}
