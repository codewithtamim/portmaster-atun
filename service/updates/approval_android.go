//go:build android

package updates

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/mgr"
)

func (u *Updater) holdsIntelForApproval() bool {
	return u.cfg.Name == intelUpdaterName
}

func intelProgressMetered() bool {
	return aconn.CurrentDeviceState().Metered
}

// stageIntelForApproval fetches the index and reports its size.
// Artifact bytes are downloaded only from the force-update API.
//
// This is a background check the user did not ask for, so a failed fetch leaves
// the state alone: nothing was being downloaded, and the installed lists are
// still fine. It also yields to a user's request, which reports its own state.
//
// Broken installed files are the exception. When the server still offers the
// installed version, they are reinstalled from the index just saved, without
// approval: nothing new is installed. Otherwise the state reports the failure
// instead of ready.
func (u *Updater) stageIntelForApproval(w *mgr.WorkerCtx) error {
	repair, err := u.stageIntel(w)
	if err != nil || !repair {
		return err
	}

	log.Warningf("updates/%s: repairing corrupted files", u.cfg.Name)
	// No index URLs: the index saved by stageIntel is installed again.
	if err := u.updateAndUpgrade(w, nil, false, true); err != nil {
		log.Warningf("updates/%s: repair failed: %s", u.cfg.Name, err)
	}
	// Not returned: a failed repair waits for the next check, and the state
	// already shows it.
	return nil
}

// stageIntel does the check of stageIntelForApproval under the update lock.
// It reports true when the installed files must be repaired.
func (u *Updater) stageIntel(w *mgr.WorkerCtx) (repair bool, err error) {
	if u.forcePending.Load() || !u.isUpdateRunning.SetToIf(false, true) {
		return false, nil
	}
	defer u.isUpdateRunning.UnSet()

	corrupted := u.corruptedInstallation != nil
	downloader := NewDownloader(u, u.getIndexURLsWithLock())
	if err := downloader.updateIndex(w.Ctx()); err != nil {
		if corrupted {
			u.setStagedState(ProgressFailed)
		}
		return false, fmt.Errorf("stage intel index: %w", err)
	}

	u.indexLock.Lock()
	current := u.index
	u.indexLock.Unlock()
	if current != nil {
		// Any refusal counts as nothing to approve: updateAndUpgrade applies
		// this same check, so the approved download would not happen anyway.
		if err := current.ShouldUpgradeTo(downloader.index); err != nil {
			switch {
			case corrupted && errors.Is(err, ErrSameIndex) && sameIndexVersion(current, downloader.index):
				return true, nil
			case corrupted:
				log.Warningf("updates/%s: cannot repair corrupted files: %s", u.cfg.Name, err)
				u.setStagedState(ProgressFailed)
				return false, nil
			case !errors.Is(err, ErrSameIndex):
				log.Warningf("updates/%s: not offering update: %s", u.cfg.Name, err)
			}
			u.setStagedState(ProgressReady)
			return false, nil
		}
	}

	total := pendingArtifactBytes(w.Ctx(), downloader.index)
	if u.forcePending.Load() {
		return false, nil
	}
	u.downloadedBytes.Store(0)
	u.totalBytes.Store(total)
	u.totalStaged.Store(true)
	u.setStagedState(ProgressAwaitingApproval)
	log.Infof("updates/%s: waiting for approval of %d bytes", u.cfg.Name, total)
	return false, nil
}

// sameIndexVersion reports whether both indexes are the same release.
// ErrSameIndex alone also covers an older index served from a stale cache. An
// index rebuilt from the installed files has no real publish date.
func sameIndexVersion(current, fetched *Index) bool {
	return current.Name == fetched.Name && current.versionNum != nil && fetched.versionNum != nil &&
		current.versionNum.Equal(fetched.versionNum) &&
		(current.isLocallyGenerated || current.Published.Equal(fetched.Published))
}

// setStagedState reports a background check's result, unless newer work owns
// the state: a user's request, or a filter-list import that is still running.
func (u *Updater) setStagedState(state string) {
	if u.forcePending.Load() || u.cacheActive.Load() {
		return
	}
	u.setProgressState(state)
}

func pendingArtifactBytes(ctx context.Context, index *Index) int64 {
	if index == nil {
		return 0
	}
	client := http.Client{Timeout: 15 * time.Second}
	var total int64
	for _, artifact := range index.Artifacts {
		if artifact == nil || len(artifact.URLs) == 0 {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, artifact.URLs[0], nil)
		if err != nil {
			log.Debugf("updates: failed to build HEAD request for %s: %s", artifact.URLs[0], err)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Debugf("updates: failed to size artifact %s: %s", artifact.URLs[0], err)
			continue
		}
		_ = resp.Body.Close()
		if resp.ContentLength > 0 {
			total += resp.ContentLength
		}
	}
	return total
}
