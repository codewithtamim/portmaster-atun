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
//
// When lists are already installed and intact and the network isn't metered,
// the update is downloaded right away, as the force-update API would: lists
// go stale otherwise, and only a first download (which may be large) or a
// metered network needs the user's approval. Otherwise artifact bytes are
// downloaded only from the force-update API.
//
// This is a background check the user did not ask for, so a failed fetch leaves
// the state alone while lists are installed: nothing was being downloaded, and
// the installed lists are still fine. Without installed lists the failure is
// reported, since nothing is filtered until a download succeeds. It also yields to a user's request, which reports its own state.
//
// Broken installed files are the exception. When the server still offers the
// installed version, they are reinstalled from the index just saved, without
// approval: nothing new is installed. Otherwise the state reports the failure
// instead of ready.
func (u *Updater) stageIntelForApproval(w *mgr.WorkerCtx) error {
	repair, update, err := u.stageIntel(w)
	if err != nil {
		return err
	}
	if update {
		log.Infof("updates/%s: downloading update on an unmetered network", u.cfg.Name)
		err := u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
		if err != nil && !errors.Is(err, ErrSameIndex) && !errors.Is(err, ErrNoUpdateAvailable) {
			// Not returned: the manager would retry at once; the next check retries.
			log.Warningf("updates/%s: automatic update failed: %s", u.cfg.Name, err)
		}
		return nil
	}
	if !repair {
		return nil
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
// It reports whether the installed files must be repaired, and whether the
// staged update may be downloaded without asking.
func (u *Updater) stageIntel(w *mgr.WorkerCtx) (repair, update bool, err error) {
	if u.forcePending.Load() || !u.isUpdateRunning.SetToIf(false, true) {
		return false, false, nil
	}
	defer u.isUpdateRunning.UnSet()

	corrupted := u.corruptedInstallation != nil
	u.indexLock.Lock()
	installed := u.index != nil && len(u.index.Artifacts) > 0
	u.indexLock.Unlock()
	downloader := NewDownloader(u, u.getIndexURLsWithLock())
	if err := downloader.updateIndex(w.Ctx()); err != nil {
		// With no lists installed there is nothing to fall back on, so the
		// failure is shown and can be retried.
		if corrupted || !installed {
			u.setStagedState(ProgressFailed)
		}
		return false, false, fmt.Errorf("stage intel index: %w", err)
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
				return true, false, nil
			case corrupted:
				log.Warningf("updates/%s: cannot repair corrupted files: %s", u.cfg.Name, err)
				u.setStagedState(ProgressFailed)
				return false, false, nil
			case !errors.Is(err, ErrSameIndex):
				log.Warningf("updates/%s: not offering update: %s", u.cfg.Name, err)
			}
			u.setStagedState(ProgressReady)
			return false, false, nil
		}
	}

	if current != nil && !corrupted && !intelProgressMetered() {
		return false, true, nil
	}

	total := pendingArtifactBytes(w.Ctx(), downloader.index)
	if u.forcePending.Load() {
		return false, false, nil
	}
	u.downloadedBytes.Store(0)
	u.totalBytes.Store(total)
	u.totalStaged.Store(true)
	u.setStagedState(ProgressAwaitingApproval)
	log.Infof("updates/%s: waiting for approval of %d bytes", u.cfg.Name, total)
	return false, false, nil
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
