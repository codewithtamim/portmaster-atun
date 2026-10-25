//go:build android

package base

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/safing/portmaster/base/database/storage/bbolt"
	"github.com/safing/portmaster/base/database/storage/pebble"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils"
)

func init() {
	DefaultDatabaseStorageType = "pebble"

	// The cache is DNS records and intel, all fetched again if lost. An fsync
	// per cached DNS answer would cost battery for nothing.
	pebble.UnsyncedDatabases["cache"] = true
}

// coreStorageType returns pebble, migrating an older bbolt core database first.
func coreStorageType() string {
	return migratedStorageType("core")
}

// cacheStorageType returns pebble, migrating an older bbolt cache database first.
func cacheStorageType() string {
	return migratedStorageType("cache")
}

// migratedStorageType returns the backend for the named database. A bbolt
// database left by an earlier build is copied into pebble, then removed.
//
// INVARIANT: bbolt is only deleted after pebble holds a complete, flushed
// copy. If the copy fails, this run keeps using bbolt and the next start
// tries again, so a failed migration never loses settings.
func migratedStorageType(name string) string {
	dbDir := filepath.Join(module.instance.DataDir(), "databases", name)
	bboltDir := filepath.Join(dbDir, "bbolt")
	pebbleDir := filepath.Join(dbDir, "pebble")

	if !utils.PathExists(filepath.Join(bboltDir, "db.bbolt")) {
		return "pebble"
	}
	if utils.PathExists(filepath.Join(pebbleDir, "db")) {
		// A migration finished but its cleanup did not. Pebble is authoritative.
		removeOldBBolt(name, bboltDir)
		return "pebble"
	}

	copied, err := migrateToPebble(name, bboltDir, pebbleDir)
	if err != nil {
		log.Errorf("base: failed to migrate %s database to pebble, using bbolt until the next start: %s", name, err)
		return "bbolt"
	}
	log.Infof("base: migrated %d records of the %s database from bbolt to pebble", copied, name)
	removeOldBBolt(name, bboltDir)
	return "pebble"
}

// migrateToPebble copies the bbolt database into a temporary directory and
// renames it into place, so pebbleDir only ever holds a complete database.
func migrateToPebble(name, bboltDir, pebbleDir string) (int, error) {
	tmpDir := pebbleDir + ".migrating"
	// Left over from a run that died mid-copy.
	if err := os.RemoveAll(tmpDir); err != nil {
		return 0, fmt.Errorf("clear %s: %w", tmpDir, err)
	}

	src, err := bbolt.NewBBolt(name, bboltDir)
	if err != nil {
		return 0, fmt.Errorf("open bbolt: %w", err)
	}
	copied, err := pebble.Import(name, tmpDir, src)
	if closeErr := src.Shutdown(); err == nil && closeErr != nil {
		err = fmt.Errorf("close bbolt: %w", closeErr)
	}
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return copied, err
	}

	// pebbleDir can exist without a database: the database module creates the
	// directory before it opens the store.
	if err := os.RemoveAll(pebbleDir); err != nil {
		return copied, fmt.Errorf("clear %s: %w", pebbleDir, err)
	}
	if err := os.Rename(tmpDir, pebbleDir); err != nil {
		return copied, fmt.Errorf("move pebble into place: %w", err)
	}
	return copied, nil
}

// removeOldBBolt deletes a bbolt database whose records now live in pebble.
func removeOldBBolt(name, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Warningf("base: failed to remove migrated bbolt %s database: %s", name, err)
	}
}
