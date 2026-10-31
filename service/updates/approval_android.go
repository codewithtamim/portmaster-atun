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
func (u *Updater) stageIntelForApproval(w *mgr.WorkerCtx) error {
	if u.forcePending.Load() || !u.isUpdateRunning.SetToIf(false, true) {
		return nil
	}
	defer u.isUpdateRunning.UnSet()

	downloader := NewDownloader(u, u.getIndexURLsWithLock())
	if err := downloader.updateIndex(w.Ctx()); err != nil {
		return fmt.Errorf("stage intel index: %w", err)
	}

	u.indexLock.Lock()
	current := u.index
	u.indexLock.Unlock()
	if current != nil {
		// Any refusal counts as nothing to approve: updateAndUpgrade applies
		// this same check, so the approved download would not happen anyway.
		if err := current.ShouldUpgradeTo(downloader.index); err != nil {
			if !errors.Is(err, ErrSameIndex) {
				log.Warningf("updates/%s: not offering update: %s", u.cfg.Name, err)
			}
			u.setStagedState(ProgressReady)
			return nil
		}
	}

	total := pendingArtifactBytes(w.Ctx(), downloader.index)
	if u.forcePending.Load() {
		return nil
	}
	u.downloadedBytes.Store(0)
	u.totalBytes.Store(total)
	u.totalStaged.Store(true)
	u.setStagedState(ProgressAwaitingApproval)
	log.Infof("updates/%s: waiting for approval of %d bytes", u.cfg.Name, total)
	return nil
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
