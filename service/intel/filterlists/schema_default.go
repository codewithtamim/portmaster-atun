//go:build !android

package filterlists

// checkListIndexSchema accepts every list index, as upstream Portmaster does.
func checkListIndexSchema(*ListIndexFile) error { return nil }
