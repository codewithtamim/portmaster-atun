//go:build !android

package docks

import "github.com/safing/portmaster/spn/terminal"

// adjustExpansionOpts keeps the terminal defaults.
func adjustExpansionOpts(_ *terminal.TerminalOpts) {}
