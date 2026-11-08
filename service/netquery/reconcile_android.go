//go:build android

package netquery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/netquery/orm"
	"github.com/safing/portmaster/service/network"
)

// reconcileEvery is how often rows still marked active are checked against the
// connections the core actually has open.
const reconcileEvery = 30 * time.Second

// reconcileBatch caps the IDs per UPDATE statement.
const reconcileBatch = 400

var lastReconcile time.Time

// reconcileActive ends rows that are still marked active although their
// connection is gone or has ended.
//
// netquery learns about an ended connection from a database update, and the
// database drops updates when netquery's feed is full. Under a flood of
// connections the end of many was lost, so their rows stayed active for good:
// counted as live connections and left out of History.
func reconcileActive(ctx context.Context, db *Database) {
	if time.Since(lastReconcile) < reconcileEvery {
		return
	}
	lastReconcile = time.Now()

	var rows []struct {
		ID string `sqlite:"id"`
	}
	// IP connections only: DNS requests are kept apart and end when answered.
	if err := db.Execute(ctx, "SELECT id FROM main.connections WHERE active = TRUE AND type = 'ip'", orm.WithResult(&rows)); err != nil {
		log.Debugf("netquery: failed to list active connections: %s", err)
		return
	}

	var stale []string
	for _, row := range rows {
		if !connectionLive(row.ID) {
			stale = append(stale, row.ID)
		}
	}
	if len(stale) == 0 {
		return
	}

	for start := 0; start < len(stale); start += reconcileBatch {
		end := min(start+reconcileBatch, len(stale))
		if err := endRows(ctx, db, stale[start:end]); err != nil {
			log.Warningf("netquery: failed to end stale connections: %s", err)
			return
		}
	}
	log.Infof("netquery: ended %d connections whose end was not recorded", len(stale))
}

// connectionLive reports whether the core still has the connection of a
// netquery row open. Row IDs are the process key and the connection ID, and on
// Android the process key is "uid-<n>".
func connectionLive(rowID string) bool {
	connID := rowID
	if rest, ok := strings.CutPrefix(rowID, "uid-"); ok {
		if _, after, found := strings.Cut(rest, "-"); found {
			connID = after
		}
	}
	return network.ConnectionStillOpen(connID)
}

// endRows marks the given rows ended in the live database and in history.
func endRows(ctx context.Context, db *Database, ids []string) error {
	params := make(map[string]any, len(ids))
	names := make([]string, len(ids))
	for i, id := range ids {
		name := fmt.Sprintf(":id%d", i)
		names[i] = name
		params[name] = id
	}
	in := strings.Join(names, ", ")

	for _, table := range []string{"main.connections", "history.connections"} {
		sql := "UPDATE " + table + " SET active = FALSE, ended = COALESCE(ended, datetime('now')) " +
			"WHERE active = TRUE AND id IN (" + in + ")"
		if err := db.ExecuteWrite(ctx, sql, orm.WithNamedArgs(params)); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	return nil
}

// repairStaleHistory ends rows that an earlier run left active in history.
// Nothing can be live when the core starts, so every active row there is a
// connection whose end was never recorded.
func repairStaleHistory(ctx context.Context, db *Database) {
	err := db.ExecuteWrite(ctx,
		"UPDATE history.connections SET active = FALSE, ended = COALESCE(ended, started) WHERE active = TRUE")
	if err != nil {
		log.Warningf("netquery: failed to end stale history rows: %s", err)
	}
}
