//go:build !android

package compat

import (
	"github.com/safing/portmaster/base/notifications"
	"github.com/safing/portmaster/service/process"
)

// shownByPlatform is false: desktop dedupes with the notifications module.
func shownByPlatform(*appIssue, *process.Process, string) bool { return false }

func showNotification(n *notifications.Notification) {
	notifications.Notify(n)
}
