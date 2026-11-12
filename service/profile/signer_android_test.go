//go:build android

package profile

import (
	"testing"

	"github.com/safing/portmaster/base/utils/aconn"
)

func withFakeSignerStore(t *testing.T) map[string]string {
	t.Helper()
	store := map[string]string{}
	saves := 0
	oldLoad, oldStore, oldSave := loadAndroidSigner, storeAndroidSigner, saveResetProfile
	loadAndroidSigner = func(key string) (string, error) { return store[key], nil }
	storeAndroidSigner = func(key, signer string) error { store[key] = signer; return nil }
	saveResetProfile = func(*Profile) error { saves++; return nil }
	t.Cleanup(func() { loadAndroidSigner, storeAndroidSigner, saveResetProfile = oldLoad, oldStore, oldSave })
	return store
}

func signerTestProfile() *Profile {
	return &Profile{
		ID:     "app",
		Source: SourceLocal,
		Config: map[string]interface{}{"filter": map[string]interface{}{"defaultAction": "permit"}},
	}
}

func TestCheckAndroidSigner(t *testing.T) { //nolint:paralleltest // swaps package storage
	store := withFakeSignerStore(t)
	p := signerTestProfile()
	key := androidSignerKey(p)

	// An app without a known signer never changes anything.
	if reset, err := CheckAndroidSigner(p, &aconn.AppInfo{}); reset || err != nil || len(store) != 0 {
		t.Fatalf("empty signer: reset=%v err=%v store=%v", reset, err, store)
	}

	// First sight: the signer is adopted, settings kept.
	if reset, err := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "AA"}); reset || err != nil {
		t.Fatalf("adopt: reset=%v err=%v", reset, err)
	}
	if store[key] != "aa" || len(p.Config) == 0 {
		t.Fatalf("adopt: store=%v config=%v", store, p.Config)
	}

	// Same signer: nothing happens.
	if reset, _ := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "aa"}); reset {
		t.Fatal("same signer reset the profile")
	}

	// Key rotation: the old key is in the lineage, settings kept.
	if reset, _ := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "bb", SignerHistory: []string{"aa", "bb"}}); reset {
		t.Fatal("rotated key reset the profile")
	}
	if store[key] != "bb" || len(p.Config) == 0 {
		t.Fatalf("rotation: store=%v config=%v", store, p.Config)
	}

	// A different developer: settings cleared and flagged.
	reset, err := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "cc", SignerHistory: []string{"cc"}})
	if !reset || err != nil {
		t.Fatalf("new signer: reset=%v err=%v", reset, err)
	}
	if len(p.Config) != 0 || p.Warning != SignerChangedWarning || store[key] != "cc" {
		t.Fatalf("new signer: config=%v warning=%q store=%v", p.Config, p.Warning, store)
	}

	// The old key's owner coming back is a change too.
	p.Config = map[string]interface{}{"x": true}
	if reset, _ := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "bb"}); !reset {
		t.Fatal("old signer without lineage was accepted")
	}
}

func TestCheckAndroidSignerSkipsSpecialProfiles(t *testing.T) { //nolint:paralleltest // swaps package storage
	store := withFakeSignerStore(t)
	p := signerTestProfile()
	p.ID = SystemProfileID
	if reset, _ := CheckAndroidSigner(p, &aconn.AppInfo{Signer: "aa"}); reset || len(store) != 0 {
		t.Fatalf("special profile touched: reset=%v store=%v", reset, store)
	}
}

func TestCheckSignerOnInstall(t *testing.T) { //nolint:paralleltest // swaps package lookups
	store := withFakeSignerStore(t)
	p := signerTestProfile()
	info := &aconn.AppInfo{UID: 10500, PackageName: "io.example.app", Signer: "aa"}
	oldInfo, oldFind := appInfoForUID, profileForPackage
	appInfoForUID = func(uid int) (*aconn.AppInfo, error) {
		if uid != info.UID {
			return nil, aconn.ErrUnknownApp
		}
		return info, nil
	}
	profileForPackage = func(pkg string) *Profile {
		if pkg == info.PackageName {
			return p
		}
		return nil
	}
	t.Cleanup(func() { appInfoForUID, profileForPackage = oldInfo, oldFind })

	// Installed and never connected: the signer is recorded, settings kept.
	checkSignerOnInstall(info.UID)
	if store[androidSignerKey(p)] != "aa" || len(p.Config) == 0 {
		t.Fatalf("install: store=%v config=%v", store, p.Config)
	}

	// Reinstalled by another developer: settings reset before any connection.
	info = &aconn.AppInfo{UID: 10501, PackageName: "io.example.app", Signer: "bb"}
	checkSignerOnInstall(info.UID)
	if len(p.Config) != 0 || p.Warning != SignerChangedWarning {
		t.Fatalf("other developer: config=%v warning=%q", p.Config, p.Warning)
	}

	// A removed app has no app info and changes nothing.
	p.Config = map[string]interface{}{"x": true}
	checkSignerOnInstall(99999)
	if len(p.Config) == 0 {
		t.Fatal("unknown uid changed the profile")
	}

	// Clones and shared user IDs are left to the connection-time check.
	info = &aconn.AppInfo{UID: 10502, PackageName: "io.example.app", Signer: "cc", AndroidUser: 10}
	checkSignerOnInstall(info.UID)
	info = &aconn.AppInfo{UID: 10503, PackageName: "io.example.app", Signer: "cc", SharedUserID: "shared.id"}
	checkSignerOnInstall(info.UID)
	if len(p.Config) == 0 {
		t.Fatal("clone or shared user ID reset the profile at install")
	}
}

func TestSignerIsRecordedForNewAndExistingApps(t *testing.T) { //nolint:paralleltest // swaps package lookups
	store := withFakeSignerStore(t)
	var p *Profile
	created := 0
	info := &aconn.AppInfo{UID: 10600, PackageName: "io.example.new", Signer: "aa", Label: "New"}
	oldInfo, oldFind, oldCreate := appInfoForUID, profileForPackage, createPackageProfile
	appInfoForUID = func(uid int) (*aconn.AppInfo, error) {
		if uid != info.UID {
			return nil, aconn.ErrUnknownApp
		}
		return info, nil
	}
	profileForPackage = func(pkg string) *Profile {
		if p != nil && pkg == info.PackageName {
			return p
		}
		return nil
	}
	createPackageProfile = func(pkg, _ string) (Snapshot, error) {
		created++
		p = signerTestProfile()
		return Snapshot{}, nil
	}
	t.Cleanup(func() { appInfoForUID, profileForPackage, createPackageProfile = oldInfo, oldFind, oldCreate })

	// A brand-new app gets a profile at install, and its signer is recorded.
	checkSignerOnInstall(info.UID)
	if created != 1 || p == nil || store[androidSignerKey(p)] != "aa" {
		t.Fatalf("new app: created=%d store=%v", created, store)
	}

	// The user blocks it; another developer's app with the same package
	// name is installed: the settings are reset before it connects.
	p.Config = map[string]interface{}{"filter": map[string]interface{}{"blockInternet": true}}
	info = &aconn.AppInfo{UID: 10601, PackageName: "io.example.new", Signer: "bb"}
	checkSignerOnInstall(info.UID)
	if len(p.Config) != 0 || created != 1 {
		t.Fatalf("other developer: config=%v created=%d", p.Config, created)
	}

	// Ensure with a uid records the signer of an app that existed before.
	p = signerTestProfile()
	info = &aconn.AppInfo{UID: 10602, PackageName: "io.example.new", Signer: "cc"}
	delete(store, androidSignerKey(p))
	checkSignerForPackage("io.example.new", 10602)
	if store[androidSignerKey(p)] != "cc" {
		t.Fatalf("ensure: store=%v", store)
	}

	// A uid that belongs to another package is ignored.
	checkSignerForPackage("io.example.other", 10602)
	if len(p.Config) == 0 {
		t.Fatal("ensure for another package changed the profile")
	}
}
