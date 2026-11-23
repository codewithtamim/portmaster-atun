//go:build android

package updates

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/mgr"
)

// checkFailureShownAfter is how many background checks in a row must fail
// before installed lists are reported as failing to update. One failure is
// often a passing network hiccup; the next check usually works.
const checkFailureShownAfter = 2

// checkFailureShown reports whether a failed background check of installed
// lists is shown to the user as a failed update. Only while the phone has a
// network that is not a sign-in portal: offline, a check cannot work and the
// lists going stale is reported by their age instead.
func checkFailureShown(failures int32, state aconn.DeviceState) bool {
	connected := (state.Wifi || state.Cellular) && !state.Captive
	return connected && failures >= checkFailureShownAfter
}

// A download the user approved that failed is tried again after
// approvedRetryFirst, then after twice the wait each time, up to
// approvedRetryLast. After that the hourly check keeps trying. The short
// first wait catches a network that dropped for a moment.
const (
	approvedRetryFirst = 30 * time.Second
	approvedRetryLast  = 16 * time.Minute
)

// approvalMaxAge ends an approval whose download did not finish within a day,
// for example because the app was not opened again. The user is then asked
// again for the lists of that day.
const approvalMaxAge = 24 * time.Hour

func (u *Updater) holdsIntelForApproval() bool {
	return u.cfg.Name == intelUpdaterName
}

func intelProgressMetered() bool {
	return aconn.CurrentDeviceState().Metered
}

// firstDownloadMarker is a file beside the installed lists that exists once a
// download has finished since install. It sits outside the list directory, so
// installing the lists bundled with a newer app version keeps it, and clearing
// the app's data removes it.
func (u *Updater) firstDownloadMarker() string {
	return filepath.Clean(u.cfg.Directory) + ".downloaded"
}

// downloadedSinceInstall reports whether a download has finished since
// install. Until then the lists in place, if any, are the ones bundled with
// the app.
func (u *Updater) downloadedSinceInstall() bool {
	_, err := os.Stat(u.firstDownloadMarker())
	return err == nil
}

// BuiltInListsInUse reports whether the intel lists in place, if any, are the
// ones bundled with the app, because no download has finished since install.
func (u *Updater) BuiltInListsInUse() bool {
	return u.holdsIntelForApproval() && !u.downloadedSinceInstall()
}

// InstalledPublished returns when the installed index was released. It is the
// zero time without an index, or when the index was built from local files and
// its date is only the time of that scan.
func (u *Updater) InstalledPublished() time.Time {
	u.indexLock.Lock()
	defer u.indexLock.Unlock()
	if u.index == nil || u.index.isLocallyGenerated {
		return time.Time{}
	}
	return u.index.Published
}

// noteDownloaded records that intel lists were downloaded, so later updates on
// an unmetered network no longer wait for approval. A failed write only means
// the user is asked once more.
func (u *Updater) noteDownloaded() {
	if !u.holdsIntelForApproval() || u.downloadedSinceInstall() {
		return
	}
	if err := os.WriteFile(u.firstDownloadMarker(), nil, 0o600); err != nil {
		log.Warningf("updates/%s: failed to record the first download: %s", u.cfg.Name, err)
	}
}

// approvalMarker is a file beside the installed lists that exists from the
// user's request for a download until that download ends. It is on disk so
// that the approval outlives the process: a download interrupted by a lost
// network or a killed app continues without asking again.
func (u *Updater) approvalMarker() string {
	return filepath.Clean(u.cfg.Directory) + ".approved"
}

// noteApproval records the user's request for a download. A failed write only
// means an interrupted download waits for the user again.
func (u *Updater) noteApproval() {
	u.approvedRetryDelay.Store(0)
	if err := os.WriteFile(u.approvalMarker(), nil, 0o600); err != nil {
		log.Warningf("updates/%s: failed to record the approved download: %s", u.cfg.Name, err)
	}
}

func (u *Updater) approvalPending() bool {
	info, err := os.Stat(u.approvalMarker())
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > approvalMaxAge {
		u.clearApproval()
		return false
	}
	return true
}

func (u *Updater) clearApproval() {
	u.approvedRetryDelay.Store(0)
	if err := os.Remove(u.approvalMarker()); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warningf("updates/%s: failed to clear the approved download: %s", u.cfg.Name, err)
	}
}

// availableDiskBytes returns the bytes available to an unprivileged process on
// the filesystem holding dir, or false when the filesystem cannot be read.
var availableDiskBytes = statfsAvailableBytes

func statfsAvailableBytes(dir string) (int64, bool) {
	target := filepath.Clean(dir)
	for {
		var st syscall.Statfs_t
		if err := syscall.Statfs(target, &st); err == nil {
			return int64(st.Bavail) * int64(st.Bsize), true
		}
		parent := filepath.Dir(target)
		if parent == target {
			return 0, false
		}
		target = parent
	}
}

func (u *Updater) hasStorageFor(needed int64) bool {
	if needed <= 0 {
		return true
	}
	avail, ok := availableDiskBytes(u.cfg.Directory)
	return !ok || avail >= needed
}

// bundleStorageMarker is written beside the list directory when the app could
// not unpack the bundled filter lists for lack of disk space. It holds the
// byte size of the bundle that failed to unpack.
func (u *Updater) bundleStorageMarker() string {
	return filepath.Clean(u.cfg.Directory) + ".storage"
}

func (u *Updater) bundleStorageShortfall() int64 {
	if !u.holdsIntelForApproval() {
		return 0
	}
	data, err := os.ReadFile(u.bundleStorageMarker())
	if err != nil {
		return 0
	}
	need, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || need <= 0 {
		return 0
	}
	return need
}

func (u *Updater) clearBundleStorageMarker() {
	if !u.holdsIntelForApproval() {
		return
	}
	if err := os.Remove(u.bundleStorageMarker()); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warningf("updates/%s: failed to clear the bundle storage marker: %s", u.cfg.Name, err)
	}
}

// adoptBundledIntel loads bundled lists that were unpacked after the updater
// started without an index, and otherwise reports a pending bundle storage
// shortfall so the app can ask the user to free space.
func (u *Updater) adoptBundledIntel() bool {
	if !u.holdsIntelForApproval() {
		return false
	}
	u.indexLock.Lock()
	installed := u.index != nil && len(u.index.Artifacts) > 0
	u.indexLock.Unlock()
	if installed {
		u.clearBundleStorageMarker()
		return false
	}

	indexPath := filepath.Join(u.cfg.Directory, u.cfg.IndexFile)
	if index, err := LoadIndex(indexPath, u.cfg.Platform, u.cfg.Verify); err == nil && len(index.Artifacts) > 0 {
		if verifyErr := index.VerifyArtifacts(u.cfg.Directory); verifyErr == nil {
			u.indexLock.Lock()
			u.index = index
			u.corruptedInstallation = nil
			u.indexLock.Unlock()
			u.clearBundleStorageMarker()
			u.requiredBytes.Store(0)
			u.setProgressState(ProgressRebuilding)
			u.EventResourcesUpdated.Submit(struct{}{})
			return true
		}
	}

	if need := u.bundleStorageShortfall(); need > 0 {
		u.requiredBytes.Store(need)
		u.setStagedState(ProgressStorage)
	}
	return false
}

// endApprovedAttempt ends the approval once its download is done or there is
// nothing to download, and otherwise schedules the next try. A full disk ends
// it too: trying again cannot help until space is freed, and the user is shown
// how much is needed.
func (u *Updater) endApprovedAttempt(err error) {
	switch {
	case err == nil, errors.Is(err, ErrSameIndex), errors.Is(err, ErrNoUpdateAvailable),
		errors.Is(err, syscall.ENOSPC):
		u.clearApproval()
	case u.approvalPending():
		u.retryApprovedDownload()
	}
}

// retryApprovedDownload runs the next check after the current wait, which
// doubles each time. Once the waits are used up the approval ends, so a
// failure that keeps coming back is not downloaded again every hour: the
// failure is shown and the user can try again. Nothing is scheduled while
// automatic checks are off.
func (u *Updater) retryApprovedDownload() {
	if !u.cfg.AutoCheck {
		return
	}
	delay := 2 * time.Duration(u.approvedRetryDelay.Load())
	if delay == 0 {
		delay = approvedRetryFirst
	}
	if delay > approvedRetryLast {
		log.Warningf("updates/%s: giving up on the approved download", u.cfg.Name)
		u.clearApproval()
		return
	}
	u.approvedRetryDelay.Store(int64(delay))
	log.Infof("updates/%s: trying the approved download again in %s", u.cfg.Name, delay)
	u.updateCheckWorkerMgr.Delay(delay)
}

// stageIntelForApproval fetches the index and reports its size.
//
// When lists are already installed and intact, the network isn't metered and a
// download has finished since install, the update is downloaded right away, as
// the force-update API would: lists go stale otherwise. The first download
// after install waits for the user's approval on any network, even when the
// lists bundled with the app are in place, and so does any download on a
// metered network. A download the user approved that has not finished is
// continued the same way, also when no lists are installed; on a metered
// network it waits for approval again. Otherwise artifact bytes are downloaded
// only from the force-update API.
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
		if u.approvalPending() {
			u.retryApprovedDownload()
		}
		return err
	}
	if update {
		log.Infof("updates/%s: downloading update on an unmetered network", u.cfg.Name)
		err := u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
		u.endApprovedAttempt(err)
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
	u.repairing.Store(true)
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

	if u.adoptBundledIntel() {
		return false, false, nil
	}
	corrupted := u.corruptedInstallation != nil
	u.indexLock.Lock()
	installed := u.index != nil && len(u.index.Artifacts) > 0
	u.indexLock.Unlock()
	if !installed {
		if need := u.bundleStorageShortfall(); need > 0 {
			u.requiredBytes.Store(need)
			u.setStagedState(ProgressStorage)
			return false, false, nil
		}
	}
	downloader := NewDownloader(u, u.getIndexURLsWithLock())
	if err := downloader.updateIndex(w.Ctx()); err != nil {
		// With no lists installed there is nothing to fall back on, so the
		// failure is shown and can be retried. Installed lists keep working,
		// so their failed check is shown only once it is clearly not the
		// network: see checkFailureShown.
		failures := u.backgroundCheckFailures.Add(1)
		if corrupted || !installed || checkFailureShown(failures, aconn.CurrentDeviceState()) {
			u.setStagedState(ProgressFailed)
		}
		return false, false, fmt.Errorf("stage intel index: %w", err)
	}
	u.backgroundCheckFailures.Store(0)

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
			u.clearApproval()
			u.setStagedState(ProgressReady)
			return false, false, nil
		}
	}

	updateUnasked := current != nil && !corrupted && u.downloadedSinceInstall()
	if !intelProgressMetered() && (updateUnasked || u.approvalPending()) {
		return false, true, nil
	}

	sizes, total, known := pendingArtifactBytes(w.Ctx(), downloader.index, installedChecksums(current, corrupted))
	if u.forcePending.Load() {
		return false, false, nil
	}
	if total > 0 && !u.hasStorageFor(total) {
		if !installed {
			u.requiredBytes.Store(total)
			u.setStagedState(ProgressStorage)
		}
		log.Warningf("updates/%s: not offering %d-byte download: not enough storage", u.cfg.Name, total)
		return false, false, nil
	}
	if known && total == 0 && current != nil {
		// Every file is installed already: only the index changes, which
		// costs no download to approve.
		return false, true, nil
	}
	u.stagedSizes.Store(sizes)
	u.downloadedBytes.Store(0)
	u.totalBytes.Store(total)
	// An unknown size is not kept: the download then sums the sizes its
	// responses report, so the bar still has a total to move against.
	u.totalStaged.Store(total > 0)
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

// installedChecksums returns the checksums of the installed files. The
// download copies these from disk instead of fetching them. It is empty when
// the installed files are broken, since the broken ones are fetched again.
func installedChecksums(current *Index, corrupted bool) map[string]bool {
	installed := make(map[string]bool)
	if current == nil || corrupted {
		return installed
	}
	for _, artifact := range current.Artifacts {
		if artifact != nil && artifact.SHA256 != "" {
			installed[artifact.SHA256] = true
		}
	}
	return installed
}

// pendingArtifactBytes is the download size of the index's artifacts that are
// not installed yet, by checksum and in total. known is false when the size is
// unknown: one artifact that cannot be sized, from a failed request or an
// error response, makes the whole size unknown, since a partial sum would be
// shown as the size and let the progress bar run past 100%.
func pendingArtifactBytes(ctx context.Context, index *Index, installed map[string]bool) (sizes map[string]int64, total int64, known bool) {
	if index == nil {
		return nil, 0, false
	}
	client := http.Client{Timeout: 15 * time.Second}
	sizes = make(map[string]int64)
	for _, artifact := range index.Artifacts {
		if artifact == nil || installed[artifact.SHA256] {
			continue
		}
		if len(artifact.URLs) == 0 {
			return nil, 0, false
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, artifact.URLs[0], nil)
		if err != nil {
			log.Debugf("updates: failed to build HEAD request for %s: %s", artifact.URLs[0], err)
			return nil, 0, false
		}
		resp, err := doWithLookupRetry(&client, req)
		if err != nil {
			log.Debugf("updates: failed to size artifact %s: %s", artifact.URLs[0], err)
			return nil, 0, false
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.ContentLength <= 0 {
			log.Debugf("updates: no size for artifact %s: status %d", artifact.URLs[0], resp.StatusCode)
			return nil, 0, false
		}
		sizes[artifact.SHA256] = resp.ContentLength
		total += resp.ContentLength
	}
	return sizes, total, true
}
