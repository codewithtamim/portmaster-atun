//go:build android

package netquery

import (
	"context"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/netquery/orm"
)

// compactFreePages is how many free pages (4 KiB each) the live database may
// hold before it is compacted.
const compactFreePages = 1024

// compactLiveDB gives the memory of deleted connections back to Android.
//
// The live database is in memory. SQLite keeps the pages of deleted rows on
// its free list, and the allocator under it keeps that memory, so a burst of
// DNS lookups (each one a row, kept for 10 minutes after it ends) left the app
// tens of megabytes larger for good. VACUUM copies the remaining rows into new
// pages and frees the old ones. It runs only once enough pages are free, since
// it copies every remaining row.
func compactLiveDB(ctx context.Context, db *Database) {
	var free []struct {
		Count int `sqlite:"freelist_count"`
	}
	if err := db.Execute(
		ctx,
		"PRAGMA main.freelist_count;",
		orm.WithTransient(),
		orm.WithResult(&free),
	); err != nil || len(free) != 1 {
		return
	}
	if free[0].Count < compactFreePages {
		return
	}

	if err := db.ExecuteWrite(ctx, "VACUUM main;"); err != nil {
		// A reader holding the database makes VACUUM fail. The next cleaner
		// run tries again.
		log.Debugf("netquery: failed to compact live db: %s", err)
		return
	}
	log.Debugf("netquery: compacted live db, released %d free pages", free[0].Count)
}
