//go:build android

package netquery

// platformIndexes are indexes the app's queries need on Android. The app pages
// history newest first, ordering by the started column; without an index on
// it every page sorts the whole table, which takes seconds on a phone once a
// busy day has filled history.
func platformIndexes() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS %sstarted_index ON %s (started)`,
	}
}
