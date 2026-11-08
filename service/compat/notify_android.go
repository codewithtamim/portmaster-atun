//go:build android

package compat

import (
	"sync"
	"time"

	"github.com/safing/portmaster/base/notifications"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/base/utils/anotif"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/process"
)

const secureDNSQuietPeriod = 30 * time.Second

// secureDNSSkippedIssue is an app using its own secure DNS while blocking it
// is off (the Android default): the lookups go through unfiltered.
// secureDNSBypassIssue is the blocked case.
var secureDNSSkippedIssue = &appIssue{
	id:    "compat:secure-dns-skipped-%s",
	title: "%s uses its own secure DNS",
}

// ReportSecureDNSSkipped reports an app whose own secure DNS was let through.
func ReportSecureDNSSkipped(p *process.Process) {
	module.mgr.Go("report skipped secure dns", func(_ *mgr.WorkerCtx) error {
		secureDNSSkippedIssue.notify(p)
		return nil
	})
}

var (
	secureDNSClearMu sync.Mutex
	secureDNSClear   = map[string]*time.Timer{}
)

// shownByPlatform reports whether notify has nothing left to do: Android
// delivers through anotif and never uses the notifications module. The
// secure-DNS warnings are shown at once, without the incident threshold.
func shownByPlatform(issue *appIssue, proc *process.Process, eventID string) bool {
	if !anotif.Handles() {
		return false
	}
	if deliverSecureDNSImmediately(issue, proc, eventID) {
		return true
	}
	return anotif.AlreadyDelivered(eventID)
}

// showNotification shows the warning through anotif when it handles them.
func showNotification(n *notifications.Notification) {
	// Titles and messages quote profile names, which may predate label
	// cleaning: strip what could restyle or split the text.
	n.Title = aconn.StripUnsafeText(n.Title)
	n.Message = aconn.StripUnsafeText(n.Message)
	if anotif.Handles() {
		anotif.Deliver(n.EventID, n.Title, n.Message)
		return
	}
	notifications.Notify(n)
}

// deliverSecureDNSImmediately shows the secure-DNS warning that matches what
// the firewall did: the lookups either skip filtering or were blocked.
func deliverSecureDNSImmediately(issue *appIssue, proc *process.Process, eventID string) bool {
	if proc == nil {
		return false
	}
	name := notifyAppName(proc)
	switch issue {
	case secureDNSSkippedIssue:
		anotif.Deliver(
			eventID,
			"App "+name+" uses its own secure DNS",
			"Lookups from the app "+name+" skip Portmaster. Turn off secure DNS in that app.",
		)
	case secureDNSBypassIssue:
		anotif.Deliver(
			eventID,
			"Portmaster blocked the secure DNS of app "+name,
			"Its lookups stay filtered. If the app "+name+" doesn't connect, turn off secure DNS in that app.",
		)
	default:
		return false
	}
	armSecureDNSClear(eventID)
	return true
}

// notifyAppName is the app's name as quoted in a notification. The label is
// chosen by the app, so it is cleaned and quoted, and the package name is
// added when it differs, so the text cannot pass for Portmaster's own.
func notifyAppName(proc *process.Process) string {
	label := aconn.SafeLabel(proc.Name)
	pkg := aconn.SafeLabel(proc.ExecName)
	switch {
	case label == "" && pkg == "":
		return "(unknown)"
	case label == "":
		return pkg
	case pkg == "" || pkg == label:
		return "\u201c" + label + "\u201d"
	default:
		return "\u201c" + label + "\u201d (" + pkg + ")"
	}
}

func armSecureDNSClear(eventID string) {
	secureDNSClearMu.Lock()
	defer secureDNSClearMu.Unlock()
	if timer := secureDNSClear[eventID]; timer != nil {
		timer.Stop()
	}
	secureDNSClear[eventID] = time.AfterFunc(secureDNSQuietPeriod, func() {
		anotif.Retract(eventID)
		secureDNSClearMu.Lock()
		delete(secureDNSClear, eventID)
		secureDNSClearMu.Unlock()
	})
}
