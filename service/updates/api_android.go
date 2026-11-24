//go:build android

package updates

import (
	"context"
	"errors"
	"net/http"
	"syscall"
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

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Download GeoIP",
		Description: "Marks the GeoIP databases as needed and downloads them in the background if they are missing.",
		Path:        "updates/intel/geoip",
		Write:       api.PermitUser,
		WriteMethod: http.MethodPost,
		ActionFunc:  u.handleIntelGeoIP,
	}); err != nil {
		return err
	}

	u.adoptBundledIntel()
	return nil
}

func (u *Updater) handleIntelProgress(_ *api.Request) (any, error) {
	u.adoptBundledIntel()
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

// EndFilterListCache ends a filter-list update pass, also one that found
// nothing to import, and clears the import once the cache write has finished.
func (u *Updater) EndFilterListCache() {
	u.cacheActive.Store(false)
	u.listChecksFinished.Add(1)
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
	if !u.startIntelDownload(false) {
		return "Intel update already started.", nil
	}
	return "Intel update started.", nil
}

// handleIntelGeoIP is called when the user turns on SPN, logs in or adds a
// country rule, which all need GeoIP. The download starts without asking:
// the user just chose something that needs it.
func (u *Updater) handleIntelGeoIP(_ *api.Request) (string, error) {
	u.noteGeoIPWanted()
	if u.geoIPInstalled(true) {
		return "GeoIP is installed.", nil
	}
	if !u.startIntelDownload(true) {
		// The running download may have passed GeoIP already.
		u.geoIPRequested.Store(true)
		return "GeoIP download queued.", nil
	}
	return "GeoIP download started.", nil
}

// startIntelDownload downloads the intel update in the background and reports
// false when a download is already running. With reinstall, a server release
// that is the installed one is installed again: that copies the files already
// in place and downloads only the missing ones, such as GeoIP.
func (u *Updater) startIntelDownload(reinstall bool) bool {
	if !u.forcePending.CompareAndSwap(false, true) {
		return false
	}
	u.noteApproval()
	// Set before returning, so a client polling after this request never reads
	// the state from before it.
	u.setProgressState(ProgressChecking)
	u.m.Go("force intel update", func(w *mgr.WorkerCtx) error {
		defer func() {
			u.forcePending.Store(false)
			if u.geoIPRequested.Swap(false) && !u.geoIPInstalled(true) {
				u.startIntelDownload(true)
			}
		}()

		finished := u.attemptsFinished.Load()
		err := u.waitForRunningUpdate(w.Ctx())
		if err == nil {
			if u.adoptBundledIntel() {
				u.clearApproval()
				u.attemptsFinished.Add(1)
				return nil
			}
			u.indexLock.Lock()
			installed := u.index != nil && len(u.index.Artifacts) > 0
			u.indexLock.Unlock()
			if !installed {
				if need := u.bundleStorageShortfall(); need > 0 {
					u.requiredBytes.Store(need)
					err = syscall.ENOSPC
				}
			}
		}
		if err == nil {
			err = u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
			// Only the same release is installed again, so an older index
			// served from a stale cache is never installed over a newer one.
			if reinstall && errors.Is(err, ErrSameIndex) {
				err = u.updateAndUpgrade(w, u.getIndexURLsWithLock(), true, true)
			}
		}
		// updateAndUpgrade reports nothing when it refuses to start.
		if u.attemptsFinished.Load() == finished {
			u.endProgress(err)
		}
		u.endApprovedAttempt(err)
		// Never return the error: the manager re-runs failed workers forever,
		// which would repeat a failed download at once, even on mobile data, and
		// flip the state the user sees. The outcome is in the progress state,
		// and endApprovedAttempt schedules the next try.
		if err != nil && !errors.Is(err, ErrSameIndex) && !errors.Is(err, ErrNoUpdateAvailable) {
			log.Warningf("updates/%s: update failed: %s", u.cfg.Name, err)
		}
		return nil
	})
	return true
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
