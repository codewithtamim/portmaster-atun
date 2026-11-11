//go:build android

package netenv

import (
	"testing"

	"github.com/safing/portmaster/base/utils/anotif"
)

type recordedNotice struct{ id, title string }

type noticeRecorder struct{ got []recordedNotice }

func (r *noticeRecorder) OnNotification(eventID, title, _ string) {
	r.got = append(r.got, recordedNotice{eventID, title})
}

func TestDNSCheckFailureIsPassedToTheApp(t *testing.T) {
	rec := &noticeRecorder{}
	anotif.RegisterListener(rec)
	t.Cleanup(func() {
		anotif.Retract(DNSCheckFailedEventID)
		anotif.RegisterListener(nil)
	})

	// Offline for another reason, such as no address, says nothing.
	platformOnlineStatusUpdated(StatusOffline, false)
	if len(rec.got) != 0 {
		t.Fatalf("offline without a DNS check failure was reported: %v", rec.got)
	}

	platformOnlineStatusUpdated(StatusOffline, true)
	platformOnlineStatusUpdated(StatusOffline, true)
	if len(rec.got) != 1 || rec.got[0].id != DNSCheckFailedEventID || rec.got[0].title == "" {
		t.Fatalf("want one shown notice, got %v", rec.got)
	}

	// Any later status withdraws it once, with an empty title.
	platformOnlineStatusUpdated(StatusOnline, false)
	platformOnlineStatusUpdated(StatusOnline, false)
	if len(rec.got) != 2 || rec.got[1].id != DNSCheckFailedEventID || rec.got[1].title != "" {
		t.Fatalf("want the notice withdrawn once, got %v", rec.got)
	}
}
