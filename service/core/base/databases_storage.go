//go:build !android

package base

// Desktop backends. Do not add pebble here: it is
// Android-only and would link cockroachdb's dependency tree into the Linux and
// Windows binaries without ever being opened.
import (
	_ "github.com/safing/portmaster/base/database/storage/bbolt"
	_ "github.com/safing/portmaster/base/database/storage/sqlite"
)
