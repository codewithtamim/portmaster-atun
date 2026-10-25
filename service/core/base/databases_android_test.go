//go:build android

package base

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/database/storage/bbolt"
	"github.com/safing/portmaster/base/database/storage/pebble"
	"github.com/safing/portmaster/base/utils"
	"github.com/safing/structures/dsd"
)

func TestMigrateToPebble(t *testing.T) {
	dbDir := t.TempDir()
	bboltDir := filepath.Join(dbDir, "bbolt")
	pebbleDir := filepath.Join(dbDir, "pebble")
	if err := os.MkdirAll(bboltDir, 0o700); err != nil {
		t.Fatal(err)
	}

	src, err := bbolt.NewBBolt("core", bboltDir)
	if err != nil {
		t.Fatal(err)
	}
	meta := &record.Meta{}
	meta.Update()
	r, err := record.NewWrapperFromDatabase("core", "config/setting", meta, dsd.JSON, []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Put(r); err != nil {
		t.Fatal(err)
	}
	if err := src.Shutdown(); err != nil {
		t.Fatal(err)
	}

	// The database module may already have created an empty pebble directory.
	if err := os.MkdirAll(pebbleDir, 0o700); err != nil {
		t.Fatal(err)
	}

	copied, err := migrateToPebble("core", bboltDir, pebbleDir)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 {
		t.Fatalf("copied %d, want 1", copied)
	}
	if utils.PathExists(pebbleDir + ".migrating") {
		t.Fatal("temporary directory left behind")
	}

	dst, err := pebble.NewPebble("core", pebbleDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Shutdown() }()
	if _, err := dst.Get("config/setting"); err != nil {
		t.Fatalf("setting not migrated: %s", err)
	}
}

func TestMigrateToPebbleKeepsBBoltOnFailure(t *testing.T) {
	dbDir := t.TempDir()
	bboltDir := filepath.Join(dbDir, "bbolt")
	pebbleDir := filepath.Join(dbDir, "pebble")
	// A file where the bbolt directory should be makes bbolt fail to open.
	if err := os.WriteFile(bboltDir, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := migrateToPebble("core", bboltDir, pebbleDir); err == nil {
		t.Fatal("expected an error")
	}
	if !utils.PathExists(bboltDir) {
		t.Fatal("source removed after a failed migration")
	}
	if utils.PathExists(pebbleDir) || utils.PathExists(pebbleDir+".migrating") {
		t.Fatal("partial pebble database left behind")
	}
}
