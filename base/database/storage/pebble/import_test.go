package pebble

import (
	"errors"
	"testing"
	"time"

	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/database/storage/bbolt"
	"github.com/safing/structures/dsd"
)

func putRecord(t *testing.T, put func(record.Record) (record.Record, error), key string, meta *record.Meta) {
	t.Helper()
	r, err := record.NewWrapperFromDatabase("core", key, meta, dsd.JSON, []byte(`{"key":"`+key+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := put(r); err != nil {
		t.Fatal(err)
	}
}

func TestImportFromBBolt(t *testing.T) {
	src, err := bbolt.NewBBolt("core", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Shutdown() }()

	live := &record.Meta{}
	live.Update()
	putRecord(t, src.Put, "config/a", live)
	putRecord(t, src.Put, "config/b", live)

	deleted := &record.Meta{}
	deleted.Update()
	deleted.Delete()
	putRecord(t, src.Put, "config/deleted", deleted)

	expired := &record.Meta{}
	expired.Update()
	expired.SetAbsoluteExpiry(time.Now().Add(-time.Hour).Unix())
	putRecord(t, src.Put, "config/expired", expired)

	target := t.TempDir()
	copied, err := Import("core", target, src)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 {
		t.Fatalf("copied %d records, want 2", copied)
	}

	dst, err := NewPebble("core", target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Shutdown() }()

	for _, key := range []string{"config/a", "config/b"} {
		r, err := dst.Get(key)
		if err != nil {
			t.Fatalf("get %s: %s", key, err)
		}
		if r.Meta().Modified != live.Modified {
			t.Fatalf("%s: meta not preserved", key)
		}
	}
	for _, key := range []string{"config/deleted", "config/expired"} {
		if _, err := dst.Get(key); err == nil {
			t.Fatalf("%s should not have been imported", key)
		}
	}
}

func TestImportRefusesExistingDatabase(t *testing.T) {
	src, err := bbolt.NewBBolt("core", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Shutdown() }()

	target := t.TempDir()
	existing, err := NewPebble("core", target)
	if err != nil {
		t.Fatal(err)
	}
	_ = existing.Shutdown()

	if _, err := Import("core", target, src); !errors.Is(err, ErrImportTarget) {
		t.Fatalf("got %v, want ErrImportTarget", err)
	}
}

func TestUnsyncedDatabases(t *testing.T) {
	UnsyncedDatabases["cache"] = true
	t.Cleanup(func() { delete(UnsyncedDatabases, "cache") })

	for name, wantSync := range map[string]bool{"core": true, "cache": false} {
		s, err := NewPebble(name, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		p := s.(*Pebble) //nolint:forcetypeassert // NewPebble returns *Pebble.
		if got := p.write.Sync; got != wantSync {
			t.Errorf("%s: sync = %v, want %v", name, got, wantSync)
		}
		_ = s.Shutdown()
	}
}
