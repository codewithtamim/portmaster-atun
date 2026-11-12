//go:build android

package profile

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/safing/portmaster/base/database"
	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
)

// Signer records live outside core:profiles/, so the profile hook and update
// feed never see them.
const androidSignerDBPrefix = "core:android/app-signers/"

// SignerChangedWarning is set on a profile whose app is now signed by a
// different developer, after its per-app settings were reset.
const SignerChangedWarning = "This app is now signed by a different developer than before. Its app settings were reset."

// androidSignerRecord holds the signer a profile's settings were made for.
type androidSignerRecord struct {
	record.Base
	sync.Mutex

	Signer string `json:"signer"`
}

func androidSignerKey(p *Profile) string {
	return androidSignerDBPrefix + p.ScopedID()
}

// Swappable in tests.
var (
	loadAndroidSigner  = loadAndroidSignerFromDB
	storeAndroidSigner = storeAndroidSignerInDB
	saveResetProfile   = (*Profile).Save
)

func loadAndroidSignerFromDB(key string) (string, error) {
	r, err := profileDB.Get(key)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	rec := &androidSignerRecord{}
	if r.IsWrapped() {
		if err := record.Unwrap(r, rec); err != nil {
			return "", err
		}
	} else if typed, ok := r.(*androidSignerRecord); ok {
		rec = typed
	} else {
		return "", fmt.Errorf("signer record is %T", r)
	}
	return rec.Signer, nil
}

func storeAndroidSignerInDB(key, signer string) error {
	rec := &androidSignerRecord{Signer: signer}
	rec.SetKey(key)
	return profileDB.Put(rec)
}

var androidSignerMu sync.Mutex

// CheckAndroidSigner binds a local profile to the developer that signed its
// app. A profile without a stored signer adopts the current one. The same
// signer, or a newer key that lists the stored one in its signing lineage,
// keeps the settings. Any other signer means a different app now holds the
// package name or sharedUserId: the profile's per-app settings are cleared
// and a warning is set, so the new app starts from the global defaults.
//
// It returns whether the settings were reset. An app without a known signer
// is left alone, so a failed lookup never clears anything.
func CheckAndroidSigner(p *Profile, info *aconn.AppInfo) (reset bool, err error) {
	if p == nil || info == nil || info.Signer == "" || p.Source != SourceLocal || isSpecialProfileID(p.ID) {
		return false, nil
	}
	current := strings.ToLower(info.Signer)
	key := androidSignerKey(p)

	androidSignerMu.Lock()
	defer androidSignerMu.Unlock()

	stored, err := loadAndroidSigner(key)
	if err != nil {
		return false, fmt.Errorf("load signer of %s: %w", p.ScopedID(), err)
	}
	if stored == current {
		return false, nil
	}
	if stored != "" && !info.SignedBySameDeveloper(stored) {
		if err := resetAppSettings(p); err != nil {
			return false, err
		}
		reset = true
		log.Warningf("profile: %s is now signed by a different developer, app settings reset", p.ScopedID())
	}
	// Adopt the signer on first sight, after a key rotation, or after a reset.
	if err := storeAndroidSigner(key, current); err != nil {
		return reset, fmt.Errorf("store signer of %s: %w", p.ScopedID(), err)
	}
	return reset, nil
}

// resetAppSettings clears the profile's own settings in place, so callers
// that already hold the profile use the defaults at once.
func resetAppSettings(p *Profile) error {
	p.Lock()
	hadConfig := len(p.Config) > 0
	if hadConfig {
		p.Config = make(map[string]interface{})
		p.dataParsed = false
		p.LastEdited = time.Now().Unix()
		p.Warning = SignerChangedWarning
		p.WarningLastUpdated = time.Now()
	}
	var err error
	if hadConfig {
		err = p.parseConfig()
	}
	p.Unlock()
	if !hadConfig {
		return nil
	}
	if err != nil {
		return err
	}
	return saveResetProfile(p)
}

// CleanAndroidProfileName cleans a profile name taken from an app label
// before labels were cleaned.
func CleanAndroidProfileName(p *Profile) error {
	if p == nil || p.Source != SourceLocal || isSpecialProfileID(p.ID) {
		return nil
	}
	p.RLock()
	name := p.Name
	p.RUnlock()
	clean := aconn.SafeLabel(name)
	if clean == name || clean == "" {
		return nil
	}
	return renameProfile(p, clean)
}

// Lookups used by checkSignerOnInstall, replaceable in tests.
var (
	appInfoForUID        = aconn.GetAppInfo
	profileForPackage    = findProfileByAndroidPackage
	createPackageProfile = ensureAndroidPackageProfile
)

func init() {
	aconn.SetOnAppInfoChanged(checkSignerOnInstall)
}

// checkSignerOnInstall runs the signer check when an app is installed or
// updated, not only at its first connection. Otherwise the settings kept for
// an uninstalled app stay visible for a reinstalled app from a different
// developer until it connects, and an app that never connected has no signer
// on record to compare against. A removed app has no app info and is skipped.
// Clones and shared user IDs keep their check at connection time.
func checkSignerOnInstall(uid int) {
	info, err := appInfoForUID(uid)
	if err != nil || info == nil || info.PackageName == "" || info.SharedUserID != "" || info.AndroidUser != 0 {
		return
	}
	// A new app gets its profile now, so its signer is on record before the
	// user sets anything for it.
	if profileForPackage(info.PackageName) == nil {
		if _, err := createPackageProfile(info.PackageName, aconn.SafeLabel(info.Label)); err != nil {
			log.Warningf("profile: failed to create profile for %s at install: %s", info.PackageName, err)
			return
		}
	}
	checkSigner(info)
}

// checkSignerForPackage records or checks the signer when the app ensures a
// package profile and knows the app's uid. It covers apps installed before
// Portmaster, whose profiles are created without an install event.
func checkSignerForPackage(packageName string, uid int) {
	info, err := appInfoForUID(uid)
	if err != nil || info == nil || info.PackageName != packageName || info.SharedUserID != "" || info.AndroidUser != 0 {
		return
	}
	checkSigner(info)
}

func checkSigner(info *aconn.AppInfo) {
	ensureProfileMu.Lock()
	p := profileForPackage(info.PackageName)
	ensureProfileMu.Unlock()
	if p == nil {
		return
	}
	if _, err := CheckAndroidSigner(p, info); err != nil {
		log.Warningf("profile: signer check for %s: %s", p.ScopedID(), err)
	}
}
