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

func countActive(t *testing.T, db *Database, table string) int {
	t.Helper()
	var res []struct {
		Count int `sqlite:"count"`
	}
	require.NoError(t, db.Execute(context.Background(),
		"SELECT COUNT(*) AS count FROM "+table+" WHERE active = TRUE", orm.WithResult(&res)))
	require.Len(t, res, 1)
	return res[0].Count
}

func TestReconcileEndsRowsWithoutConnection(t *testing.T) { //nolint:paralleltest // package state.
	db := newTestDatabase(t)
	ctx := context.Background()

	// Rows whose end update was lost: active, but the core has no such
	// connection. More than one batch, in both tables.
	now := time.Now()
	for i := range reconcileBatch + 50 {
		require.NoError(t, db.Save(ctx, Conn{
			ID:          fmt.Sprintf("uid-10312-6-100.127.247.245-%d-91.108.56.174-443", 20000+i),
			ProfileID:   "local/tg",
			Type:        "ip",
			Started:     now,
			Active:      true,
			ProfileName: "Telegram",
		}, true))
	}
	// A DNS row stays as it is: those are not tracked as IP connections.
	require.NoError(t, db.Save(ctx, Conn{ID: "uid-10312-dns-1", Type: "dns", Started: now, Active: true}, true))

	require.Equal(t, reconcileBatch+51, countActive(t, db, "main.connections"))
	require.Equal(t, reconcileBatch+51, countActive(t, db, "history.connections"))

	lastReconcile = time.Time{}
	reconcileActive(ctx, db)

	require.Equal(t, 1, countActive(t, db, "main.connections"))
	require.Equal(t, 1, countActive(t, db, "history.connections"))

	// Within the interval it does not run again.
	require.NoError(t, db.Save(ctx, Conn{ID: "uid-1-6-a-1-b-2", Type: "ip", Started: now, Active: true}, false))
	reconcileActive(ctx, db)
	require.Equal(t, 2, countActive(t, db, "main.connections"))
}

func TestRepairStaleHistory(t *testing.T) { //nolint:paralleltest // package state.
	db := newTestDatabase(t)
	ctx := context.Background()
	require.NoError(t, db.Save(ctx, Conn{ID: "uid-1-6-x", Type: "ip", Started: time.Now(), Active: true}, true))
	require.Equal(t, 1, countActive(t, db, "history.connections"))

	repairStaleHistory(ctx, db)

	require.Zero(t, countActive(t, db, "history.connections"))
}

func TestConnectionLiveParsesRowIDs(t *testing.T) { //nolint:paralleltest // package state.
	// No connection exists in the core, so every row is not live; the point is
	// that row IDs with and without the process key do not panic.
	for _, id := range []string{"uid-10312-6-1.2.3.4-1-5.6.7.8-443", "uid-", "6-1.2.3.4-1-5.6.7.8-443", ""} {
		require.False(t, connectionLive(id), id)
	}
}
