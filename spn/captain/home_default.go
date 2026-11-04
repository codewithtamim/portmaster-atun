//go:build !android

package captain

import "github.com/safing/portmaster/spn/terminal"

// homeTerminalOpts keeps the upstream home hub terminal options.
func homeTerminalOpts() *terminal.TerminalOpts {
	return terminal.DefaultHomeHubTerminalOpts()
}
