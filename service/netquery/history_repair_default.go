//go:build !android

package netquery

// repairHistoryFile keeps the existing behaviour outside Android: the history
// database is opened as it is.
func repairHistoryFile(string) {}
