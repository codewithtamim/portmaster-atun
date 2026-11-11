//go:build !android

package netquery

func platformSource([]DatabaseName, string) string { return "" }
