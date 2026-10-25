//go:build !android

package dbmodule

import (
	// Register the sqlite backend for the desktop builds.
	_ "github.com/safing/portmaster/base/database/storage/sqlite"
)
