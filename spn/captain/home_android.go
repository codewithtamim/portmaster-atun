//go:build android

package captain

import "github.com/safing/portmaster/spn/terminal"

// homeQueueSize is the flow-control window, in messages of up to 1500 bytes,
// of the terminal to the home hub. Every SPN route runs inside it, so it caps
// how much data can queue on the phone's link in total. The default of 50000
// (about 75 MB) let heavy browsing queue over 15 s of data, so DNS lookups
// over the SPN timed out and the home hub ping failed. 4096 messages (6 MB)
// keeps that delay to a second or two and is twice one route's window.
const homeQueueSize = 4096

func homeTerminalOpts() *terminal.TerminalOpts {
	opts := terminal.DefaultHomeHubTerminalOpts()
	opts.FlowControlSize = homeQueueSize
	return opts
}
