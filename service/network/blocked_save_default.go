//go:build !android

package network

// endRefused does nothing off Android: the OS integration reports the end of
// a connection, as upstream.
func endRefused(*Connection) {}

// skipRepeatedBlock never skips off Android: every attempt is saved, as
// upstream.
func skipRepeatedBlock(*Connection) bool { return false }
