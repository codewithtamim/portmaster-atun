package base

import (
	"github.com/safing/portmaster/base/database"
)

// Storage backends are chosen per platform by coreStorageType and
// cacheStorageType. Desktop uses an existing bbolt database, otherwise
// sqlite. Android uses pebble for both and opens bbolt only to migrate.

// Default Values (changeable for testing).
var (
	DefaultDatabaseStorageType = "sqlite"
)

func registerDatabases() error {
	// Register core database.
	_, err := database.Register(&database.Database{
		Name:        "core",
		Description: "Holds core data, such as settings and profiles",
		StorageType: coreStorageType(),
	})
	if err != nil {
		return err
	}

	// Register cache database.
	_, err = database.Register(&database.Database{
		Name:        "cache",
		Description: "Cached data, such as Intelligence and DNS Records",
		StorageType: cacheStorageType(),
	})
	if err != nil {
		return err
	}

	return nil
}
