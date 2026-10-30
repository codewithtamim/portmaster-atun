//go:build android

package pebble

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/safing/portmaster/base/database/query"
	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/structures/dsd"
)

func TestUseAfterShutdown(t *testing.T) {
	t.Parallel()

	db, err := open("core", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	live := &record.Meta{}
	live.Update()
	putRecord(t, db.Put, "config/a", live)

	if err := db.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := db.Shutdown(); err != nil {
		t.Fatalf("second Shutdown: %s", err)
	}

	if _, err := db.Get("config/a"); !errors.Is(err, errClosed) {
		t.Errorf("Get: got %v, want errClosed", err)
	}
	r, err := record.NewWrapperFromDatabase("core", "config/b", live, dsd.JSON, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Put(r); !errors.Is(err, errClosed) {
		t.Errorf("Put: got %v, want errClosed", err)
	}
	if err := db.Delete("config/a"); !errors.Is(err, errClosed) {
		t.Errorf("Delete: got %v, want errClosed", err)
	}
	if _, err := db.Query(query.New("core:"), true, true); !errors.Is(err, errClosed) {
		t.Errorf("Query: got %v, want errClosed", err)
	}
	if err := db.MaintainRecordStates(context.Background(), time.Now(), true); !errors.Is(err, errClosed) {
		t.Errorf("MaintainRecordStates: got %v, want errClosed", err)
	}
	if _, err := db.Purge(context.Background(), query.New("core:"), true, true, false); !errors.Is(err, errClosed) {
		t.Errorf("Purge: got %v, want errClosed", err)
	}
}
