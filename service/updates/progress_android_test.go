//go:build android

package updates

import (
	"errors"
	"testing"
)

func intelTestUpdater() *Updater {
	u := &Updater{}
	u.cfg.Name = intelUpdaterName
	return u
}

func progressState(u *Updater) string {
	state, _ := u.progressState.Load().(string)
	return state
}

func TestAppliedUpdateWaitsForTheListImport(t *testing.T) {
	u := intelTestUpdater()
	u.beginProgress()
	u.endProgress(nil)
	if got := progressState(u); got != ProgressRebuilding {
		t.Fatalf("state before the import began: %s", got)
	}

	u.BeginFilterListCache(10)
	if got := progressState(u); got != ProgressRebuilding {
		t.Fatalf("state during the import: %s", got)
	}
	u.EndFilterListCache()
	if got := progressState(u); got != ProgressIdle {
		t.Fatalf("state after the import: %s", got)
	}
}

func TestListPassWithNothingToImportEndsTheWait(t *testing.T) {
	u := intelTestUpdater()
	u.beginProgress()
	u.endProgress(nil)
	u.EndFilterListCache()
	if got := progressState(u); got != ProgressIdle {
		t.Fatalf("state stuck after an empty pass: %s", got)
	}
}

func TestListPassEndingBeforeTheAttemptDoesNotStick(t *testing.T) {
	u := intelTestUpdater()
	u.beginProgress()
	u.EndFilterListCache()
	u.endProgress(nil)
	if got := progressState(u); got != ProgressReady {
		t.Fatalf("state after a pass that already ended: %s", got)
	}
}

func TestImportRunningWhenTheAttemptEndsStaysRebuilding(t *testing.T) {
	u := intelTestUpdater()
	u.beginProgress()
	u.EndFilterListCache()
	u.BeginFilterListCache(10)
	u.endProgress(nil)
	if got := progressState(u); got != ProgressRebuilding {
		t.Fatalf("state with an import running: %s", got)
	}
}

func TestOtherUpdatersDoNotWaitForAnImport(t *testing.T) {
	u := &Updater{}
	u.beginProgress()
	u.endProgress(nil)
	if got := progressState(u); got != ProgressReady {
		t.Fatalf("state: %s", got)
	}
}

func TestFailedAttemptDoesNotWaitForAnImport(t *testing.T) {
	u := intelTestUpdater()
	u.beginProgress()
	u.endProgress(errors.New("download failed"))
	if got := progressState(u); got != ProgressFailed {
		t.Fatalf("state: %s", got)
	}
}
