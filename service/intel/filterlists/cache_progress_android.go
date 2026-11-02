//go:build android

package filterlists

import (
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/base/database/storage/pebble"
	"github.com/safing/portmaster/service/updates"
)

// cacheBytesRead is how many bytes of the current import have been pulled
// from the list files. The decode step fills a small channel, then waits
// on the database writer, so the count stays with the cache load.
var cacheBytesRead atomic.Int64

func beginCacheLoad(files []*updates.Artifact) {
	var total int64
	for _, file := range files {
		if file == nil {
			continue
		}
		info, err := os.Stat(file.Path())
		if err != nil {
			continue
		}
		total += info.Size()
	}
	cacheBytesRead.Store(0)
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.BeginFilterListCache(total)
}

func endCacheLoad() {
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.EndFilterListCache()
}

func noteCacheBytes(n int64) {
	if n <= 0 {
		return
	}
	done := cacheBytesRead.Add(n)
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.NoteFilterListCache(done)
}

func intelUpdater() *updates.Updater {
	if module == nil || module.instance == nil {
		return nil
	}
	return module.instance.IntelUpdates()
}

// cacheProgressReader counts bytes as the list file is decoded.
type cacheProgressReader struct {
	r io.Reader
}

func (c cacheProgressReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		noteCacheBytes(int64(n))
	}
	return n, err
}

// countCacheBytes wraps a list file so its import is counted.
func countCacheBytes(r io.Reader) io.Reader {
	return cacheProgressReader{r: r}
}

// cacheUpdatedAt records when the lists last changed, for the UI. The bloom
// filters live in memory, so every start re-imports the lists already applied;
// that re-import keeps the earlier time, or "Updated just now" would show on
// each start for lists that are days old.
func cacheUpdatedAt(ver string, now time.Time) int64 {
	if prev, err := loadCacheVersionRecord(); err == nil && prev.Version == ver && prev.LastUpdatedUnix > 0 {
		return prev.LastUpdatedUnix
	}
	return now.Unix()
}

// persistCache makes the finished import durable. The cache database skips
// the fsync per write, so without this the version record (the last write)
// is lost when Android ends the process, and every start re-imports all lists.
func persistCache() error {
	return pebble.Sync("cache")
}
