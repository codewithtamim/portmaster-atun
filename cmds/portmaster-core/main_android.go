//go:build android

package main

import (
	"github.com/spf13/cobra"

	"github.com/safing/portmaster/cmds/cmdbase"
)

// runPlatformSpecifics runs platform specific commands.
//
// Android has no iptables recovery: interception runs through the TUN
// interface, not nfqueue.
func runPlatformSpecifics(cmd *cobra.Command, args []string) {
	if printVersion {
		runFlagCmd(cmdbase.Version, cmd, args)
	}
}
