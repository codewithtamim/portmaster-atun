//go:build android

package netquery

import "fmt"

// platformSource builds the row source for a query over the live and the
// history database together. A connection is saved to both while history is
// on for it, so plain UNION has to compare whole rows, blobs included, to
// drop the copies: with a busy day in history that is a sort of every row of
// the day on every query, seconds of disk reads per statistics refresh. Here
// each connection comes from history when it is there and from the live
// database otherwise (history off for its app, or not saved yet), checked by
// id, which the primary key answers at once. Other database sets keep the
// default. An empty result means: use the default.
func platformSource(databases []DatabaseName, whereClause string) string {
	if len(databases) != 2 || !hasBoth(databases) {
		return ""
	}
	liveOnly := "WHERE id NOT IN (SELECT id FROM history.connections)"
	if whereClause != "" {
		liveOnly = whereClause + " AND id NOT IN (SELECT id FROM history.connections)"
	}
	return fmt.Sprintf(
		"SELECT * FROM %s.connections %s UNION ALL SELECT * FROM %s.connections %s",
		HistoryDatabase, whereClause, LiveDatabase, liveOnly,
	)
}

func hasBoth(databases []DatabaseName) bool {
	var live, history bool
	for _, db := range databases {
		switch db {
		case LiveDatabase:
			live = true
		case HistoryDatabase:
			history = true
		}
	}
	return live && history
}
