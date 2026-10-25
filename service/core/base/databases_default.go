//go:build !android

package base

import (
	"path/filepath"

	"github.com/safing/portmaster/base/utils"
)

// coreStorageType keeps an existing bbolt core database. New installs use
// the default backend.
func coreStorageType() string {
	storageType := DefaultDatabaseStorageType
	if utils.PathExists(filepath.Join(module.instance.DataDir(), "databases", "core", "bbolt")) {
		storageType = "bbolt"
	}

	return storageType
}

// cacheStorageType returns the storage type to use for the cache database.
func cacheStorageType() string {
	// If there is an existing cache bbolt database, use it instead.
	cacheStorageType := DefaultDatabaseStorageType
	if utils.PathExists(filepath.Join(module.instance.DataDir(), "databases", "cache", "bbolt")) {
		cacheStorageType = "bbolt"
	}

	return cacheStorageType
}
