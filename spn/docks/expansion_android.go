//go:build android

package docks

import "github.com/safing/portmaster/spn/terminal"

// expansionQueueSize is the flow-control window, in messages of up to 1500
// bytes, of an expansion terminal (one SPN route, shared by the connections
// that use it). The terminal default of 50000 preallocates about 1.2 MB of
// pointer channels per route, kept for the 15 minute idle timeout, so a
// browsing session left tens of MB behind for the GC to scan. 8192 messages
// (about 12 MB in flight) still covers several fast connections on one route.
const expansionQueueSize = 8192

func adjustExpansionOpts(opts *terminal.TerminalOpts) {
	opts.FlowControlSize = expansionQueueSize
}
