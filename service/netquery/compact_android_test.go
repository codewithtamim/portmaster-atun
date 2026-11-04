//go:build android

package netquery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/safing/portmaster/service/netquery/orm"
)

func freePages(t *testing.T, db *Database) int {
	t.Helper()

	var free []struct {
		Count int `sqlite:"freelist_count"`
	}
	require.NoError(t, db.Execute(
		context.Background(),
		"PRAGMA main.freelist_count;",
		orm.WithTransient(),
		orm.WithResult(&free),
	))
	require.Len(t, free, 1)
	return free[0].Count
}

func TestCompactLiveDBReleasesDeletedRows(t *testing.T) { //nolint:paralleltest // uses the package-level module.
	db := newTestDatabase(t)
	ctx := context.Background()

	// About as many lookups as a few minutes of busy browsing.
	ended := time.Now().Add(-time.Hour)
	for i := range 20000 {
		require.NoError(t, db.Save(ctx, Conn{
			ID:          fmt.Sprintf("dns-%d", i),
			ProfileID:   "local/com.example",
			Path:        "/data/app/com.example",
			Type:        "dns",
			Domain:      fmt.Sprintf("r%d.example.com.", i),
			ProfileName: "Example",
			Started:     ended,
			Ended:       &ended,
		}, false))
	}
	removed, err := db.Cleanup(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 20000, removed)
	require.GreaterOrEqual(t, freePages(t, db), compactFreePages)

	compactLiveDB(ctx, db)

	require.Zero(t, freePages(t, db))
}

func TestCompactLiveDBSkipsSmallFreeLists(t *testing.T) { //nolint:paralleltest // uses the package-level module.
	db := newTestDatabase(t)
	ctx := context.Background()

	ended := time.Now().Add(-time.Hour)
	for i := range 500 {
		require.NoError(t, db.Save(ctx, Conn{
			ID:      fmt.Sprintf("dns-%d", i),
			Type:    "dns",
			Domain:  fmt.Sprintf("r%d.example.com.", i),
			Started: ended,
			Ended:   &ended,
		}, false))
	}
	_, err := db.Cleanup(ctx, time.Now())
	require.NoError(t, err)
	before := freePages(t, db)
	require.Positive(t, before)

	compactLiveDB(ctx, db)

	require.Equal(t, before, freePages(t, db))
}
