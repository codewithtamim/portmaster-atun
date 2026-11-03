//go:build android

package updates

import (
	"errors"
	"io"
	"os"
	"sync/atomic"
	"syscall"
)

// Progress states reported to API clients.
const (
	ProgressIdle = "idle"
	// ProgressChecking is a user's update request before any artifact bytes
	// move: waiting for a background check, then fetching the index.
	ProgressChecking         = "checking"
	ProgressDownloading      = "downloading"
	ProgressFailed           = "failed"
	ProgressReady            = "ready"
	ProgressStorage          = "storage"
	ProgressAwaitingApproval = "awaiting-approval"
	// ProgressRebuilding is the filter-list import into the local cache.
	ProgressRebuilding = "rebuilding"

	intelUpdaterName = "Portmaster Intel"
)

// progressTracker is the Android download progress the app UI polls while
// the user approves and watches a filter-list download.
type progressTracker struct {
	progressState               atomic.Value // string, one of the Progress* states
	downloadedBytes             atomic.Int64
	totalBytes                  atomic.Int64
	totalStaged                 atomic.Bool // totalBytes was set by stageIntelForApproval
	requiredBytes               atomic.Int64
	cacheLoadedBytes            atomic.Int64
	cacheTotalBytes             atomic.Int64
	cacheActive                 atomic.Bool
	filterListsReadyCheck       atomic.Value // func() bool
	filterListsLastUpdatedCheck atomic.Value // func() int64
	forcePending                atomic.Bool  // a user's update request is in progress
	attemptsFinished            atomic.Int64
}

// Progress is byte-level intel download status for HTTP clients.
type Progress struct {
	State            string `json:"state"`
	DownloadedBytes  int64  `json:"downloadedBytes"`
	TotalBytes       int64  `json:"totalBytes"`
	FilterListsReady bool   `json:"filterListsReady"`
	LastUpdatedUnix  int64  `json:"lastUpdatedUnix"`
	// RequiredBytes is how much free space the unpack needs when State is storage.
	RequiredBytes int64 `json:"requiredBytes"`
	// Metered is set on Android when the active network is metered.
	Metered bool `json:"metered,omitempty"`
	// CacheLoadedBytes is how much of the list files has been read into the
	// local cache while State is rebuilding.
	CacheLoadedBytes int64 `json:"cacheLoadedBytes,omitempty"`
	// CacheTotalBytes is the on-disk size of the list files being imported.
	CacheTotalBytes int64 `json:"cacheTotalBytes,omitempty"`
	// AttemptsFinished counts ended update attempts. A client compares it across
	// a request to learn when that attempt ended, and State is then its outcome.
	AttemptsFinished int64 `json:"attemptsFinished"`
}

// SetFilterListsReadyCheck lets filterlists report whether blooms are loaded.
func (u *Updater) SetFilterListsReadyCheck(fn func() bool) {
	u.filterListsReadyCheck.Store(fn)
}

// SetFilterListsLastUpdatedCheck lets filterlists report when lists were applied.
func (u *Updater) SetFilterListsLastUpdatedCheck(fn func() int64) {
	u.filterListsLastUpdatedCheck.Store(fn)
}

// Progress returns a snapshot of the current intel download.
func (u *Updater) Progress() Progress {
	state, _ := u.progressState.Load().(string)
	if state == "" {
		state = ProgressIdle
	}
	ready := false
	if fn, ok := u.filterListsReadyCheck.Load().(func() bool); ok && fn != nil {
		ready = fn()
	}
	var lastUpdated int64
	if fn, ok := u.filterListsLastUpdatedCheck.Load().(func() int64); ok && fn != nil {
		lastUpdated = fn()
	}
	return Progress{
		State:            state,
		DownloadedBytes:  u.downloadedBytes.Load(),
		TotalBytes:       u.totalBytes.Load(),
		FilterListsReady: ready,
		LastUpdatedUnix:  lastUpdated,
		RequiredBytes:    u.requiredBytes.Load(),
		Metered:          intelProgressMetered(),
		CacheLoadedBytes: u.cacheLoadedBytes.Load(),
		CacheTotalBytes:  u.cacheTotalBytes.Load(),
		AttemptsFinished: u.attemptsFinished.Load(),
	}
}

// beginProgress starts a new attempt. The approved total from
// stageIntelForApproval is kept, so the bar does not jump when artifact
// requests report their own sizes. Without one, the total is summed from
// the artifact responses. The state moves to downloading once readBody sees
// artifact bytes: most checks end at the index and never get there.
func (u *Updater) beginProgress() {
	u.downloadedBytes.Store(0)
	if !u.totalStaged.Load() {
		u.totalBytes.Store(0)
	}
	u.requiredBytes.Store(0)
	u.progressState.Store(ProgressChecking)
}

// noteStorageShortfall records how many bytes the unpack needed when the
// disk filled up. Clients show that number instead of a generic failure.
func (u *Updater) noteStorageShortfall() {
	if u.requiredBytes.Load() > 0 {
		return
	}
	need := u.totalBytes.Load()
	if need <= 0 {
		need = u.downloadedBytes.Load()
	}
	if need > 0 {
		u.requiredBytes.Store(need)
	}
}

func (u *Updater) setProgressState(state string) {
	u.progressState.Store(state)
}

func (u *Updater) addDownloaded(n int64) {
	if n > 0 {
		u.downloadedBytes.Add(n)
	}
}

func (u *Updater) addTotal(n int64) {
	if n <= 0 || u.totalStaged.Load() {
		return
	}
	u.totalBytes.Add(n)
}

// countingReader counts bytes as they are read so the UI can show progress
// while artifacts still stream in.
type countingReader struct {
	r   io.Reader
	add func(int64)
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.add != nil {
		c.add(int64(n))
	}
	return n, err
}

// endProgress maps the result of updateAndUpgrade to the state the UI shows.
// The staged total is used up by the download it was staged for.
//
// The counter moves last, so a client that sees it change reads the outcome.
func (u *Updater) endProgress(err error) {
	defer u.attemptsFinished.Add(1)
	u.totalStaged.Store(false)

	switch {
	case err == nil:
		if u.cacheActive.Load() {
			u.setProgressState(ProgressRebuilding)
		} else {
			u.setProgressState(ProgressReady)
		}
	case errors.Is(err, ErrSameIndex), errors.Is(err, ErrNoUpdateAvailable), errors.Is(err, ErrActionRequired):
		if u.cacheActive.Load() {
			u.setProgressState(ProgressRebuilding)
		} else {
			u.setProgressState(ProgressIdle)
		}
	case errors.Is(err, syscall.ENOSPC):
		u.noteStorageShortfall()
		u.setProgressState(ProgressStorage)
	default:
		u.setProgressState(ProgressFailed)
	}
}

// countExistingFile counts an artifact that is already on disk as downloaded,
// so the bar reaches 100% when only some artifacts changed.
func (d *Downloader) countExistingFile(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	d.u.addTotal(info.Size())
	d.u.addDownloaded(info.Size())
}
