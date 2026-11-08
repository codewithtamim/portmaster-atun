//go:build android

package process

import (
	"context"
	"testing"

	"github.com/safing/portmaster/base/utils/aconn"
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
