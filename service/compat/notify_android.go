//go:build android

package compat

import (
	"sync"
	"time"

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

// deliverSecureDNSImmediately shows the secure-DNS warning that matches what
// the firewall did: the lookups either skip filtering or were blocked.
func deliverSecureDNSImmediately(issue *appIssue, proc *process.Process, eventID string) bool {
	if proc == nil {
		return false
	}
	name := proc.Name
	if name == "" {
		name = "An app"
	}
	switch issue {
	case secureDNSSkippedIssue:
		anotif.Deliver(
			eventID,
			name+" uses its own secure DNS",
			"Lookups from "+name+" skip Portmaster. Turn off secure DNS in that app.",
		)
	case secureDNSBypassIssue:
		anotif.Deliver(
			eventID,
			"Portmaster blocked "+name+"'s own secure DNS",
			"Its lookups stay filtered. If "+name+" doesn't connect, turn off secure DNS in that app.",
		)
	default:
		return false
	}
	armSecureDNSClear(eventID)
	return true
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
