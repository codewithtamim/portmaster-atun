//go:build android

package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBundle writes an unsigned index with version and the given files into
// dir, as the app's bundle installer leaves them.
func writeBundle(t *testing.T, dir, version string, files map[string]string) {
	t.Helper()
	index := &Index{Name: intelUpdaterName, Version: version, Published: testPublished}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(content))
		index.Artifacts = append(index.Artifacts, &Artifact{
			Filename: name,
			SHA256:   hex.EncodeToString(sum[:]),
			Version:  "20261101.0.7-basic.1",
		})
	}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newVerifyingIntel(t *testing.T, dir string) *Updater {
	t.Helper()
	_, trustStore := newSigningKey(t, "release key")
	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         dir,
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		Verify:            trustStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestBasicBundleIndexIsTrusted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "basic lists"})

	u := newVerifyingIntel(t, dir)
	if u.corruptedInstallation != nil {
		t.Fatalf("basic bundle reported as corrupted: %s", u.corruptedInstallation)
	}
	if u.index == nil || u.index.isLocallyGenerated || u.index.Version != "20261124.8.7-basic" {
		t.Fatalf("basic bundle index not loaded: %+v", u.index)
	}
}

// Only the basic bundle may skip the signature; any other unsigned index is
// still a broken installation.
func TestUnsignedReleaseIndexIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7", map[string]string{"base.dsdl": "lists"})

	u := newVerifyingIntel(t, dir)
	if u.corruptedInstallation == nil {
		t.Fatal("an unsigned release index was trusted")
	}
}

func TestBasicBundleFromTheFutureIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "basic lists"})
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	index.Published = time.Now().Add(24 * time.Hour)
	if data, err = json.Marshal(&index); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if u := newVerifyingIntel(t, dir); u.corruptedInstallation == nil {
		t.Fatal("a basic bundle dated in the future was trusted")
	}
}

func TestOtherPrereleaseTagIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basicx", map[string]string{"base.dsdl": "lists"})

	if u := newVerifyingIntel(t, dir); u.corruptedInstallation == nil {
		t.Fatal("an unsigned index with another tag was trusted")
	}
}

func TestGeoIPMissingOnlyOnceWanted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "basic lists"})
	u := newVerifyingIntel(t, dir)

	if u.geoIPMissing() {
		t.Fatal("GeoIP reported missing before anything asked for it")
	}
	u.noteGeoIPWanted()
	if !u.geoIPMissing() {
		t.Fatal("GeoIP not reported missing after it was asked for")
	}
	for _, name := range geoIPFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("geo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if u.geoIPMissing() {
		t.Fatal("GeoIP reported missing while installed")
	}
}

func TestReleaseIsOfferedOverBasicBundle(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "basic lists"})
	u := newVerifyingIntel(t, dir)

	release := &Index{Name: intelUpdaterName, Version: "20261124.8.7", Published: testPublished.Add(time.Second)}
	if err := release.init(currentPlatform); err != nil {
		t.Fatal(err)
	}
	if err := u.index.ShouldUpgradeTo(release); err != nil {
		t.Fatalf("release not offered over the basic bundle: %s", err)
	}
}

func TestGeoIPIsSkippedUntilWanted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "basic lists"})
	u := newVerifyingIntel(t, dir)
	geo := &Artifact{Filename: "geoipv4.mmdb"}

	if !u.skipsArtifact(geo) {
		t.Fatal("GeoIP is downloaded before anything asked for it")
	}
	if u.skipsArtifact(&Artifact{Filename: "base.dsdl"}) {
		t.Fatal("filter lists are skipped")
	}

	u.noteGeoIPWanted()
	if u.skipsArtifact(geo) {
		t.Fatal("GeoIP is still skipped after it was asked for")
	}
}

// Users who have GeoIP from an older bundle keep getting its updates.
func TestInstalledGeoIPKeepsUpdating(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{
		"base.dsdl":    "basic lists",
		"geoipv6.mmdb": "geo",
	})
	u := newVerifyingIntel(t, dir)

	if u.skipsArtifact(&Artifact{Filename: "geoipv4.mmdb"}) {
		t.Fatal("GeoIP updates stop while a GeoIP file is installed")
	}
}

// A release lists GeoIP; while it is not wanted its absence is not a broken
// installation, and once it is wanted it is.
func TestMissingGeoIPIsBrokenOnlyOnceWanted(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "20261124.8.7-basic", map[string]string{"base.dsdl": "lists"})
	u := newVerifyingIntel(t, dir)

	index := &Index{Name: intelUpdaterName, Version: "20261124.8.7", Published: testPublished}
	index.Artifacts = append(index.Artifacts, u.index.Artifacts...)
	index.Artifacts = append(index.Artifacts, &Artifact{Filename: "geoipv4.mmdb", SHA256: "00"})

	if err := u.verifyInstalled(index); err != nil {
		t.Fatalf("missing GeoIP counted as broken before it was wanted: %s", err)
	}
	u.noteGeoIPWanted()
	if err := u.verifyInstalled(index); err == nil {
		t.Fatal("missing GeoIP not noticed after it was wanted")
	}
}
