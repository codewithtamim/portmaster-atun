//go:build android

package docks

import "github.com/safing/portmaster/spn/terminal"

// expansionQueueSize is the flow-control window, in messages of up to 1500
// bytes, of an expansion terminal (one SPN route, shared by the connections
// that use it). The terminal default of 50000 preallocates about 1.2 MB of
// pointer channels per route, kept for the 15 minute idle timeout. A large
// window also lets a busy route queue seconds of data on the phone's link,
// which delays DNS and control messages behind it. 2048 messages (3 MB)
// still covers several fast connections on one route.
const expansionQueueSize = 2048

func adjustExpansionOpts(opts *terminal.TerminalOpts) {
	opts.FlowControlSize = expansionQueueSize
}
