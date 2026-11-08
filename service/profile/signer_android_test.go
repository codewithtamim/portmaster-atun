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
