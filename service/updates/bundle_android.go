//go:build android

package updates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/safing/portmaster/base/log"
)

// basicBundleTag marks the version of an index the app bundled with a smaller
// set of filter lists. Such an index cannot carry Safing's signature, since
// its lists differ from the release.
const basicBundleTag = "basic"

// geoIPFiles are the intel files downloaded only once something needs them:
// SPN, an account or a country rule. They are most of the intel download.
var geoIPFiles = []string{"geoipv4.mmdb", "geoipv6.mmdb"}

// loadInstalledIndex loads the installed index. The intel updater also
// accepts the unsigned index of the basic bundle the app ships. It lives in
// the app's private storage, written there from the APK, and anything that
// can write there could also replace the files a signed index describes. Its
// version sorts below the release it came from, so the release is offered
// over it.
func (u *Updater) loadInstalledIndex(path string) (*Index, error) {
	index, err := LoadIndex(path, u.cfg.Platform, u.cfg.Verify)
	if err == nil || u.cfg.Name != intelUpdaterName || u.cfg.Verify == nil {
		return index, err
	}
	unsigned, unsignedErr := LoadIndex(path, u.cfg.Platform, nil)
	if unsignedErr != nil || !isBasicBundle(unsigned) {
		return index, err
	}
	return unsigned, nil
}

// isBasicBundle also requires a release date that has passed: a date in the
// future would make the engine refuse every release offered over it.
func isBasicBundle(index *Index) bool {
	return index.versionNum != nil && index.versionNum.Prerelease() == basicBundleTag &&
		!index.Published.After(time.Now())
}

// verifyInstalled checks the installed files against index. A GeoIP file
// that is not wanted yet may be missing.
func (u *Updater) verifyInstalled(index *Index) error {
	for _, artifact := range index.Artifacts {
		path := filepath.Join(u.cfg.Directory, artifact.Filename)
		if u.skipsArtifact(artifact) {
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		if err := CheckSHA256SumFile(path, artifact.SHA256); err != nil {
			return fmt.Errorf("verify %s: %w", artifact.Filename, err)
		}
	}
	return nil
}

// skipsArtifact reports whether an update leaves artifact out: a GeoIP file
// nothing has asked for. Once a GeoIP file is installed it stays wanted, so
// it keeps getting updates.
func (u *Updater) skipsArtifact(artifact *Artifact) bool {
	return u.cfg.Name == intelUpdaterName && slices.Contains(geoIPFiles, artifact.Filename) && !u.geoIPWanted()
}

// geoIPMarker is a file beside the installed lists that exists once GeoIP was
// asked for. Clearing the app's data removes it.
func (u *Updater) geoIPMarker() string {
	return filepath.Clean(u.cfg.Directory) + ".geoip"
}

func (u *Updater) geoIPWanted() bool {
	if _, err := os.Stat(u.geoIPMarker()); err == nil {
		return true
	}
	return u.geoIPInstalled(false)
}

// geoIPInstalled reports whether any GeoIP file is installed, or with all
// set, whether every one is.
func (u *Updater) geoIPInstalled(all bool) bool {
	for _, name := range geoIPFiles {
		_, err := os.Stat(filepath.Join(u.cfg.Directory, name))
		switch {
		case err == nil && !all:
			return true
		case err != nil && all:
			return false
		}
	}
	return all
}

// geoIPMissing reports whether GeoIP is wanted but not every file of it is
// installed.
func (u *Updater) geoIPMissing() bool {
	return u.cfg.Name == intelUpdaterName && u.geoIPWanted() && !u.geoIPInstalled(true)
}

// noteGeoIPWanted records that GeoIP is needed from now on. A failed write
// only means the files are not kept up to date until they are installed.
func (u *Updater) noteGeoIPWanted() {
	if err := os.WriteFile(u.geoIPMarker(), nil, 0o600); err != nil {
		log.Warningf("updates/%s: failed to record that GeoIP is needed: %s", u.cfg.Name, err)
	}
}
