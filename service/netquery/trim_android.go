//go:build android

package netquery

import (
	"context"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/netquery/orm"
)

// maxEndedLiveRows caps the ended connections kept in the live database,
// which is held in memory. Ended rows normally stay ten minutes; after a
// flood that is every row of those minutes. The newest stay, so the Monitor
// still lists recent connections of apps whose history is off; everything
// else is read from the history database on disk.
const maxEndedLiveRows = 5000

const oldEndedLiveRows = `SELECT id FROM main.connections WHERE ended IS NOT NULL
	ORDER BY ended DESC LIMIT -1 OFFSET :keep`

func trimLiveDB(ctx context.Context, db *Database) {
	trimLiveDBTo(ctx, db, maxEndedLiveRows)
}

func trimLiveDBTo(ctx context.Context, db *Database, keep int) {
	var ended []struct {
		Count int `sqlite:"count"`
	}
	if err := db.Execute(
		ctx,
		"SELECT COUNT(*) AS count FROM main.connections WHERE ended IS NOT NULL",
		orm.WithTransient(),
		orm.WithResult(&ended),
	); err != nil || len(ended) != 1 || ended[0].Count <= keep {
		return
	}

	args := orm.WithNamedArgs(map[string]interface{}{":keep": keep})
	// Bandwidth rows first: they are selected by the connections to delete.
	if err := db.ExecuteWrite(ctx, "DELETE FROM main.bandwidth WHERE conn_id IN ("+oldEndedLiveRows+");", args); err != nil {
		log.Debugf("netquery: failed to trim bandwidth of old live rows: %s", err)
		return
	}
	if err := db.ExecuteWrite(ctx, "DELETE FROM main.connections WHERE id IN ("+oldEndedLiveRows+");", args); err != nil {
		log.Debugf("netquery: failed to trim old live rows: %s", err)
	}
}
