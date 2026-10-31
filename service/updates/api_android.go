//go:build android

package updates

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/mgr"
)

// registerIntelAPI adds the endpoints the app UI uses to approve, start and
// watch a filter-list download.
func (u *Updater) registerIntelAPI() error {
	// The progress endpoints describe the filter-list download only.
	if u.cfg.Name != intelUpdaterName {
		return nil
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Intel Download Progress",
		Description: "Returns byte progress for the in-flight intel / filter-list download.",
		Path:        "updates/intel/progress",
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		StructFunc:  u.handleIntelProgress,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Force Intel Update",
		Description: "Starts an intel / filter-list download in the background.",
		Path:        "updates/intel/update",
		Write:       api.PermitUser,
		WriteMethod: http.MethodPost,
		ActionFunc:  u.handleIntelForceUpdate,
	}); err != nil {
		return err
	}

	return nil
}

func (u *Updater) handleIntelProgress(_ *api.Request) (any, error) {
	return u.Progress(), nil
}

// BeginFilterListCache marks the import of list files into the local cache.
// Download and failure states stay as they are so a transfer is not hidden.
func (u *Updater) BeginFilterListCache(total int64) {
	u.cacheActive.Store(true)
	u.cacheLoadedBytes.Store(0)
	if total > 0 {
		u.cacheTotalBytes.Store(total)
	}
	state, _ := u.progressState.Load().(string)
	switch state {
	case ProgressChecking, ProgressDownloading, ProgressFailed, ProgressStorage, ProgressAwaitingApproval:
		return
	default:
		u.setProgressState(ProgressRebuilding)
	}
}

// NoteFilterListCache records how many list-file bytes have been read.
func (u *Updater) NoteFilterListCache(loaded int64) {
	if loaded < 0 {
		return
	}
	total := u.cacheTotalBytes.Load()
	if total > 0 && loaded > total {
		loaded = total
	}
	u.cacheLoadedBytes.Store(loaded)
}

// EndFilterListCache clears the import once the cache write has finished.
func (u *Updater) EndFilterListCache() {
	u.cacheActive.Store(false)
	state, _ := u.progressState.Load().(string)
	if state == ProgressRebuilding {
		u.setProgressState(ProgressIdle)
	}
	u.cacheLoadedBytes.Store(0)
	u.cacheTotalBytes.Store(0)
}

func (u *Updater) handleIntelForceUpdate(_ *api.Request) (string, error) {
	// Manual (user-initiated) updates are always allowed, even on metered
	// (mobile data) networks: the automatic path defers on metered, but a
	// manual retry must proceed.
	// HTTP must not wait for the full download: the dashboard polls progress.
	if !u.forcePending.CompareAndSwap(false, true) {
		return "Intel update already started.", nil
	}
	// Set before returning, so a client polling after this request never reads
	// the state from before it.
	u.setProgressState(ProgressChecking)
	u.m.Go("force intel update", func(w *mgr.WorkerCtx) error {
		defer u.forcePending.Store(false)

		finished := u.attemptsFinished.Load()
		err := u.waitForRunningUpdate(w.Ctx())
		if err == nil {
			err = u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
		}
		// updateAndUpgrade reports nothing when it refuses to start.
		if u.attemptsFinished.Load() == finished {
			u.endProgress(err)
		}
		// Never return the error: the manager re-runs failed workers forever,
		// which would repeat a failed download unasked, even on mobile data, and
		// flip the state the user sees. The outcome is in the progress state.
		if err != nil && !errors.Is(err, ErrSameIndex) && !errors.Is(err, ErrNoUpdateAvailable) {
			log.Warningf("updates/%s: update failed: %s", u.cfg.Name, err)
		}
		return nil
	})
	return "Intel update started.", nil
}

// waitForRunningUpdate waits out a background check, which holds the update
// lock for its index fetch and artifact sizing. Without this, updateAndUpgrade
// refuses to start and a user's tap does nothing.
func (u *Updater) waitForRunningUpdate(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for u.isUpdateRunning.IsSet() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return nil
}
