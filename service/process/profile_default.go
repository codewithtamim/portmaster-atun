//go:build !android

package process

import (
	"context"
	"runtime"
	"strings"

	"github.com/safing/portmaster/service/profile"
)

// getSpecialProfileID returns the special profile ID for the process, if any.
func (p *Process) getSpecialProfileID() (specialProfileID string) {
	// Check if we need a special profile.
	switch p.Pid {
	case UnidentifiedProcessID:
		specialProfileID = profile.UnidentifiedProfileID
	case UnsolicitedProcessID:
		specialProfileID = profile.UnsolicitedProfileID
	case SystemProcessID:
		specialProfileID = profile.SystemProfileID
	case ownPID:
		specialProfileID = profile.PortmasterProfileID
	default:
		// Check if this is another Portmaster component.
		if p.IsPortmasterUi(context.Background()) {
			specialProfileID = profile.PortmasterAppProfileID
		}
		// Check if this is the system resolver.
		switch runtime.GOOS {
		case "windows": //nolint:goconst // same GOOS literal as the rest of this package
			// Depending on the OS version System32 may be capitalized or not.
			if (p.Path == `C:\Windows\System32\svchost.exe` ||
				p.Path == `C:\Windows\system32\svchost.exe`) &&
				// This comes from the windows tasklist command and should be pretty consistent.
				(profile.KeyAndValueInTags(p.Tags, "svchost", "Dnscache") ||
					// As an alternative in case of failure, we try to match the svchost.exe service parameter.
					strings.Contains(p.CmdLine, "-s Dnscache")) {
				specialProfileID = profile.SystemResolverProfileID
			}
		case "linux":
			switch p.Path {
			case "/lib/systemd/systemd-resolved",
				"/usr/lib/systemd/systemd-resolved",
				"/lib64/systemd/systemd-resolved",
				"/usr/lib64/systemd/systemd-resolved",
				"/usr/bin/nscd",
				"/usr/sbin/nscd",
				"/usr/bin/dnsmasq",
				"/usr/sbin/dnsmasq":
				specialProfileID = profile.SystemResolverProfileID
			}
		}
	}

	return specialProfileID
}

// hasIdentifiedOwner returns whether the process is a real process and not one
// of the special unidentified placeholders.
func (p *Process) hasIdentifiedOwner() bool {
	switch p.Pid {
	case UndefinedProcessID, UnidentifiedProcessID, UnsolicitedProcessID:
		return false
	default:
		return true
	}
}

// ownerID returns the identity of the process owner on this platform.
func (p *Process) ownerID() int {
	return p.Pid
}
