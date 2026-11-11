//go:build android

package netquery

import (
	"os"

	"github.com/safing/portmaster/base/log"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// repairHistoryFile sets aside a connection history file that SQLite cannot
// read at all, so the engine starts with an empty history instead of not
// starting.
//
// History is a convenience; the filtering must not depend on it. A file whose
// header or schema is damaged (a torn write, a full disk, a restored backup)
// made the engine stop with "file is not a database", which left protection off
// with nothing to say why. Only the two result codes that mean "this is not a
// usable database" move the file. Any other error, such as a locked file or a
// missing permission, leaves it where it is so good history is never discarded
// over a transient problem.
//
// The damaged file is kept next to the new one as history.db.corrupt, replacing
// an older copy, so a person can still hand it to support.
func repairHistoryFile(historyFile string) {
	if _, err := os.Stat(historyFile); err != nil {
		return
	}

	conn, err := sqlite.OpenConn(historyFile, sqlite.OpenReadOnly)
	if err == nil {
		defer conn.Close() //nolint:errcheck // read-only probe
		err = sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM sqlite_master;", nil)
	}
	if err == nil {
		return
	}

	switch sqlite.ErrCode(err).ToPrimary() { //nolint:exhaustive // only these two mean unusable
	case sqlite.ResultNotADB, sqlite.ResultCorrupt:
	default:
		return
	}

	log.Warningf("netquery: history database is damaged (%s); starting with an empty history", err)
	quarantined := historyFile + ".corrupt"
	_ = os.Remove(quarantined)
	if err := os.Rename(historyFile, quarantined); err != nil {
		log.Errorf("netquery: failed to set the damaged history database aside: %s", err)
		_ = os.Remove(historyFile)
	}
	_ = os.Remove(historyFile + "-wal")
	_ = os.Remove(historyFile + "-shm")
}
