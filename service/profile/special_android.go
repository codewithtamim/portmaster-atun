//go:build android

package profile

import (
	"errors"

	"github.com/safing/portmaster/base/database"
	"github.com/safing/portmaster/base/log"
)

// ensurePlatformProfiles creates the profile for traffic that can't be linked
// to an app, if it doesn't exist yet. It is normally made on the first such
// connection, which left the Apps screen without its row on a fresh install:
// the user could not set a rule for that traffic ahead of time.
func ensurePlatformProfiles() {
	_, err := getProfile(MakeScopedID(SourceLocal, UnidentifiedProfileID))
	switch {
	case err == nil:
		return
	case !errors.Is(err, database.ErrNotFound):
		log.Warningf("profile: failed to look up the unidentified profile: %s", err)
		return
	}

	profile := createSpecialProfile(UnidentifiedProfileID, "")
	updateSpecialProfileMetadata(profile, "")
	if err := profile.Save(); err != nil {
		log.Warningf("profile: failed to create the unidentified profile: %s", err)
	}
}
