//go:build android

package dbmodule

import (
	// Register the pebble backend for the Android build.
	_ "github.com/safing/portmaster/base/database/storage/pebble"
)
