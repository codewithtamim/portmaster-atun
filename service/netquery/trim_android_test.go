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

func liveIDs(t *testing.T, db *Database) map[string]bool {
	t.Helper()
	var rows []struct {
		ID string `sqlite:"id"`
	}
	require.NoError(t, db.Execute(context.Background(), "SELECT id FROM main.connections", orm.WithResult(&rows)))
	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		ids[row.ID] = true
	}
	return ids
}

func TestTrimLiveDBKeepsNewestEndedAndAllOpenRows(t *testing.T) { //nolint:paralleltest // uses the package-level module.
	db := newTestDatabase(t)
	ctx := context.Background()

	base := time.Now().Add(-time.Hour)
	for i := range 30 {
		ended := base.Add(time.Duration(i) * time.Second)
		require.NoError(t, db.Save(ctx, Conn{
			ID: fmt.Sprintf("ended-%02d", i), Type: "dns", Started: ended, Ended: &ended,
		}, false))
	}
	for i := range 5 {
		require.NoError(t, db.Save(ctx, Conn{
			ID: fmt.Sprintf("open-%d", i), Type: "ip", Started: base, Active: true,
		}, false))
	}

	trimLiveDBTo(ctx, db, 10)

	ids := liveIDs(t, db)
	require.Len(t, ids, 15)
	for i := range 5 {
		require.True(t, ids[fmt.Sprintf("open-%d", i)], "open rows stay")
	}
	for i := 20; i < 30; i++ {
		require.True(t, ids[fmt.Sprintf("ended-%02d", i)], "the newest ended rows stay")
	}
}

func TestTrimLiveDBLeavesSmallTablesAlone(t *testing.T) { //nolint:paralleltest // uses the package-level module.
	db := newTestDatabase(t)
	ctx := context.Background()
	ended := time.Now().Add(-time.Minute)
	for i := range 8 {
		require.NoError(t, db.Save(ctx, Conn{ID: fmt.Sprintf("c-%d", i), Type: "dns", Started: ended, Ended: &ended}, false))
	}
	trimLiveDBTo(ctx, db, 10)
	require.Len(t, liveIDs(t, db), 8)
}
