//go:build android

package pebble

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	crdb "github.com/cockroachdb/pebble/v2"

	"github.com/safing/portmaster/base/database/query"
	"github.com/safing/portmaster/base/database/storage"
)

// ErrImportTarget is returned when Import would write into an existing database.
var ErrImportTarget = errors.New("pebble: import target already holds a database")

// Import copies every live record of src into a new pebble database at
// location and returns how many it copied. Expired and deleted records are
// skipped, as src.Query skips them.
//
// It is meant for a one-time migration, so the copy itself is unsynced and
// the result is flushed to disk once at the end. On error the target is left
// partial: the caller must delete location and keep using src.
func Import(name, location string, src storage.Interface) (copied int, err error) {
	if _, statErr := os.Stat(filepath.Join(location, "db")); statErr == nil {
		return 0, ErrImportTarget
	}

	dst, err := open(name, location, crdb.NoSync)
	if err != nil {
		return 0, fmt.Errorf("open target: %w", err)
	}
	defer func() {
		if closeErr := dst.Shutdown(); err == nil && closeErr != nil {
			err = fmt.Errorf("close target: %w", closeErr)
		}
	}()

	it, err := src.Query(query.New(name+":"), true, true)
	if err != nil {
		return 0, fmt.Errorf("query source: %w", err)
	}

	batch, errs := dst.PutMany(false)
	for r := range it.Next {
		select {
		case batch <- r:
			copied++
		case putErr := <-errs:
			// PutMany stops reading after its first error.
			it.Cancel()
			return copied, fmt.Errorf("write target: %w", putErr)
		}
	}
	close(batch)
	if putErr := <-errs; putErr != nil {
		return copied, fmt.Errorf("write target: %w", putErr)
	}
	if queryErr := it.Err(); queryErr != nil {
		return copied, fmt.Errorf("read source: %w", queryErr)
	}

	// The unsynced copy only lives in the WAL and memtable so far.
	if err := dst.db.Flush(); err != nil {
		return copied, fmt.Errorf("flush target: %w", err)
	}

	return copied, nil
}
