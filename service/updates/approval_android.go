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
func (u *Updater) stageIntelForApproval(w *mgr.WorkerCtx) error {
	if !u.isUpdateRunning.SetToIf(false, true) {
		return nil
	}
	defer u.isUpdateRunning.UnSet()

	downloader := NewDownloader(u, u.getIndexURLsWithLock())
	if err := downloader.updateIndex(w.Ctx()); err != nil {
		u.setProgressState(ProgressFailed)
		return fmt.Errorf("stage intel index: %w", err)
	}

	u.indexLock.Lock()
	current := u.index
	u.indexLock.Unlock()
	if current != nil {
		err := current.ShouldUpgradeTo(downloader.index)
		if errors.Is(err, ErrSameIndex) {
			u.setProgressState(ProgressReady)
			return nil
		}
	}

	total := pendingArtifactBytes(w.Ctx(), downloader.index)
	u.downloadedBytes.Store(0)
	u.totalBytes.Store(total)
	u.totalStaged.Store(true)
	u.setProgressState(ProgressAwaitingApproval)
	log.Infof("updates/%s: waiting for approval of %d bytes", u.cfg.Name, total)
	return nil
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
