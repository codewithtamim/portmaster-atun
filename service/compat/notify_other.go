//go:build !android

package compat

import "github.com/safing/portmaster/service/process"

func deliverSecureDNSImmediately(_ *appIssue, _ *process.Process, _ string) bool {
	return false
}
